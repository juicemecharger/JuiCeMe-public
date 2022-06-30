package main

import (
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
	version                  = "0.1.2"
)

var log *logrus.Logger
var centralSystem ocpp16.CentralSystem

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
	//moved up
	//configKey := "MeterValueSampleInterval"
	//configValue := "10"

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
	cp := handler.chargePoints[chargePointID]
	cp.Power.L1 = 0
	cp.Power.L2 = 0
	cp.Power.L3 = 0
	cp.Power.Total = 0
	cp.Currents.L1 = 0
	cp.Currents.L2 = 0
	cp.Currents.L3 = 0
	//done, all Load Management values reset

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
	//Set to safe Charge Limit
	time.Sleep(waitinterval * time.Second)
	success := handler.SetConfig(chargePointID, "DlmOperatorPhase1Limit", "8")
	if success {
		success = handler.SetConfig(chargePointID, "DlmOperatorPhase2Limit", "8")
	}
	if success {
		success = handler.SetConfig(chargePointID, "DlmOperatorPhase3Limit", "8")
	}
	if !success {
		log.Println("Error whilst setting safe current!!!!!!!!!!!!!!!!!!!!!")
	}
	///

}

// Start function
func main() {
	// Load config from ENV
	var listenPort = defaultListenPort
	// Prepare OCPP 1.6 central system
	centralSystem = setupCentralSystem()
	// Support callbacks for all OCPP 1.6 profiles
	handler := &CentralSystemHandler{chargePoints: map[string]*ChargePointState{}}
	centralSystem.SetCoreHandler(handler)
	centralSystem.SetLocalAuthListHandler(handler)
	centralSystem.SetFirmwareManagementHandler(handler)
	centralSystem.SetReservationHandler(handler)
	centralSystem.SetRemoteTriggerHandler(handler)
	centralSystem.SetSmartChargingHandler(handler)
	// Add handlers for dis/connection of charge points
	centralSystem.SetNewChargePointHandler(func(chargePoint ocpp16.ChargePointConnection) {
		handler.chargePoints[chargePoint.ID()] = &ChargePointState{Connectors: map[int]*ConnectorInfo{}, Transactions: map[int]*TransactionInfo{}}
		log.WithField("client", chargePoint.ID()).Info("new charge point connected")
		go setupRoutine(chargePoint.ID(), handler)
	})
	centralSystem.SetChargePointDisconnectedHandler(func(chargePoint ocpp16.ChargePointConnection) {
		log.WithField("client", chargePoint.ID()).Info("charge point disconnected")
		delete(handler.chargePoints, chargePoint.ID())
	})
	ocppj.SetLogger(log.WithField("logger", "ocppj"))
	//ws.Server.Errors()

	// Run central system
	log.Infof("starting central system on port %v", listenPort)
	go handler.Listen(version)
	centralSystem.Start(listenPort, "/{ws}")
	log.Info("stopped central system")
}

func init() {
	log = logrus.New()
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: false})
	// Set this to DebugLevel if you want to retrieve verbose logs from the ocppj and websocket layers
	log.SetLevel(logrus.DebugLevel)
}
