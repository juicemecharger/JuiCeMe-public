package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/localauth"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/lorenzodonini/ocpp-go/ocppj"
	"github.com/sirupsen/logrus"
)

const (
	defaultListenPort        = 8887
	defaultHeartbeatInterval = 60
	waitinterval             = 5
	version                  = "0.1.6"
	authlistfilename         = "ident.json"
	centralsystemfilename    = "persistence.json"
	debugvalue               = false
)

var log *logrus.Logger
var centralSystem ocpp16.CentralSystem
var identity ident

type ident struct {
	Cards map[string]authIdStruct `json:"cards"`
	MACs  map[string]authIdStruct `json:"macs"`
}

type authIdStruct struct {
	TXList         map[string]TransactionInfo `json:"tx_list"`
	Authorized     bool                       `json:"authorized"`
	EnergyCharged  int64                      `json:"energy_charged"`
	CurrentSession int64                      `json:"current_session"`
}

func setupCentralSystem() ocpp16.CentralSystem {
	return ocpp16.NewCentralSystem(nil, nil)
}

// Run for every connected Charge Point, pushing config
func setupRoutine(chargePointID string, handler *CentralSystemHandler) {
	var e error
	var KeyMeterValuesSampledData = "MeterValuesSampledData"
	var KeyMeterValueSampleInterval = "MeterValueSampleInterval"
	var MeterSampleInterval = "10"

	//Wait
	time.Sleep(waitinterval * time.Second)
	// Change meter sampling values time
	callback1 := func(confirmation *core.ChangeConfigurationConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, core.ChangeConfigurationFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == core.ConfigurationStatusNotSupported {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for unsupported key: %v", KeyMeterValueSampleInterval)
		} else if confirmation.Status == core.ConfigurationStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for readonly key: %v", KeyMeterValueSampleInterval)
		} else {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("updated configuration for key %v to: %v", KeyMeterValueSampleInterval, MeterSampleInterval)
		}
	}
	e = centralSystem.ChangeConfiguration(chargePointID, callback1, KeyMeterValueSampleInterval, MeterSampleInterval)
	if e != nil {
		logDefault(chargePointID, localauth.GetLocalListVersionFeatureName).Errorf("couldn't send message: %v", e)
		return
	}

	// Setting Meter Sampling Data (Supported by JuiceMe, maximum data)
	const ValuePreferedMeterValuesSampleData = "Current.Import.L1,Current.Import.L2,Current.Import.L3,Current.Offered,Energy.Active.Import.Register,Power.Active.Import"
	time.Sleep(waitinterval * time.Second)
	callback1v2 := func(confirmation *core.ChangeConfigurationConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, core.ChangeConfigurationFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == core.ConfigurationStatusNotSupported {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for unsupported key: %v", KeyMeterValueSampleInterval)
		} else if confirmation.Status == core.ConfigurationStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for readonly key: %v", KeyMeterValueSampleInterval)
		} else {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("updated configuration for key %v to: %v", KeyMeterValueSampleInterval, MeterSampleInterval)
		}
	}
	e = centralSystem.ChangeConfiguration(chargePointID, callback1v2, KeyMeterValuesSampledData, ValuePreferedMeterValuesSampleData)
	if e != nil {
		logDefault(chargePointID, localauth.GetLocalListVersionFeatureName).Errorf("couldn't send message: %v", e)
		return
	}

	//set all value 0 for Power
	cp := handler.ChargePoints[chargePointID]
	cp.Power.L1 = 0
	cp.Power.L2 = 0
	cp.Power.L3 = 0
	cp.Power.Total = 0
	cp.Currents.L1 = 0
	cp.Currents.L2 = 0
	cp.Currents.L3 = 0
	//done, all Load values reset

	// Wait
	time.Sleep(waitinterval * time.Second)
	// Trigger a heartbeat message
	callback2 := func(confirmation *remotetrigger.TriggerMessageConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v triggered successfully", core.HeartbeatFeatureName)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v trigger was rejected", core.HeartbeatFeatureName)
		}
	}
	e = centralSystem.TriggerMessage(chargePointID, callback2, core.HeartbeatFeatureName)
	if e != nil {
		logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("couldn't send message: %v", e)
		return
	}

	// Wait
	time.Sleep(waitinterval * time.Second)
	// Trigger a diagnostics status notification
	callback3 := func(confirmation *remotetrigger.TriggerMessageConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v triggered successfully", firmware.GetDiagnosticsFeatureName)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v trigger was rejected", firmware.GetDiagnosticsFeatureName)
		}
	}
	e = centralSystem.TriggerMessage(chargePointID, callback3, firmware.DiagnosticsStatusNotificationFeatureName)
	if e != nil {
		logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("couldn't send message: %v", e)
		return
	}
	//Start Set to safe Charge Limit, so in case something breaks whilst dlm its doing its stuff we don't trip a breaker, lulz
	time.Sleep(waitinterval * time.Second)
	success := handler.SetConfig(chargePointID, "DlmOperatorPhase1Limit", "6")
	if success {
		success = handler.SetConfig(chargePointID, "DlmOperatorPhase2Limit", "6")
	}
	if success {
		success = handler.SetConfig(chargePointID, "DlmOperatorPhase3Limit", "6")
	}
	if !success {
		log.Println("Error whilst setting safe current!!!!!!!!!!!!!!!!!!!!!") //maybe something here to stop autorization on that guy until its manually solved
	}
	handler.ChargePointsInitialized[chargePointID] = true
	///End Set to safe Charge Limit 0

}

// Start function
func main() {
	//Persistence of cards/EVCCID(Prefix "MAC")
	authFile, _ := ioutil.ReadFile(authlistfilename)
	_ = json.Unmarshal(authFile, &identity)
	//persistence for centralSystem
	handler := &CentralSystemHandler{ChargePoints: map[string]*ChargePointState{}, Groups: map[string]*Group{}, GroupsInitialized: map[string]bool{}, ChargePointsInitialized: map[string]bool{}, debug: debugvalue, Transactions: map[int]*TransactionInfo{}}

	//Leave commented out for now until we have a file
	centralSystemFile, _ := ioutil.ReadFile(centralsystemfilename)
	_ = json.Unmarshal(centralSystemFile, &handler)
	// Load config from const
	var listenPort = defaultListenPort
	// Prepare OCPP 1.6 central system
	centralSystem = setupCentralSystem()
	// Support callbacks for all OCPP 1.6 profiles
	centralSystem.SetCoreHandler(handler)
	centralSystem.SetLocalAuthListHandler(handler)
	centralSystem.SetFirmwareManagementHandler(handler)
	centralSystem.SetReservationHandler(handler)
	centralSystem.SetRemoteTriggerHandler(handler)
	centralSystem.SetSmartChargingHandler(handler)
	// Add handlers for dis/connection of charge points
	centralSystem.SetNewChargePointHandler(func(chargePoint ocpp16.ChargePointConnection) {
		if !handler.ChargePointsInitialized[chargePoint.ID()] {
			handler.ChargePoints[chargePoint.ID()] = &ChargePointState{Connectors: map[int]*ConnectorInfo{}}
		}
		log.WithField("client", chargePoint.ID()).Info("new charge point connected")
		groupdid := string([]rune(chargePoint.ID())[0])
		log.Println(groupdid)
		if !handler.GroupsInitialized[groupdid] {
			handler.Groups[groupdid] = &Group{Chargers: map[string]string{}, MaxL1: 32, MaxL2: 32, MaxL3: 32}
			handler.Groups[groupdid].Chargers[chargePoint.ID()] = "true"
			handler.GroupsInitialized[groupdid] = true
		} else {
			handler.Groups[groupdid].Chargers[chargePoint.ID()] = "true"
		}
		go setupRoutine(chargePoint.ID(), handler)
	})
	//DisconnectHandler
	centralSystem.SetChargePointDisconnectedHandler(func(chargePoint ocpp16.ChargePointConnection) {
		log.WithField("client", chargePoint.ID()).Info("charge point disconnected")
		//delete(handler.chargePoints, chargePoint.ID())
		handler.ChargePoints[chargePoint.ID()].Status = core.ChargePointStatusUnavailable
		groupdid := string([]rune(chargePoint.ID())[0])
		delete(handler.Groups[groupdid].Chargers, chargePoint.ID())
	})
	ocppj.SetLogger(log.WithField("logger", "ocppj"))
	//ws.Server.Errors()

	// Run central system
	log.Infof("starting central system on port %v", listenPort)
	go handler.Listen(version)
	go handler.dlmstart()
	centralSystem.Start(listenPort, "/{ws}")
	log.Info("stopped central system")
	defer func() {
		fmt.Println("Saving Files to Disk (Persistence)")
		authlistjson, _ := json.MarshalIndent(identity, "", " ")
		centralSystemjson, _ := json.MarshalIndent(handler, "", " ")
		log.Println(authlistjson)
		log.Println(centralSystemjson)
		_ = ioutil.WriteFile(authlistfilename, authlistjson, 0644)
		_ = ioutil.WriteFile(centralsystemfilename, centralSystemjson, 0644)
	}()
}

func init() {
	log = logrus.New()
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: false})
	// Set this to DebugLevel if you want to retrieve verbose logs from the ocppj and websocket layers
	log.SetLevel(logrus.DebugLevel)
}
