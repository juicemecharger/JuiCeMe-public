package main

import (
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/localauth"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/reservation"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
	"github.com/lorenzodonini/ocpp-go/ocppj"
	"github.com/sirupsen/logrus"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
)

const (
	defaultListenPort        = 8887
	defaultHeartbeatInterval = 60
)

var log *logrus.Logger
var centralSystem ocpp16.CentralSystem

func setupCentralSystem() ocpp16.CentralSystem {
	return ocpp16.NewCentralSystem(nil, nil)
}

// Run for every connected Charge Point, to simulate some functionality
func setupRoutine(chargePointID string, handler *CentralSystemHandler) {
	//// Wait for some time
	//time.Sleep(5 * time.Second)
	// Get current local list version
	//cb3 := func(confirmation *localauth.GetLocalListVersionConfirmation, err error) {
	//	if err != nil {
	//		logDefault(chargePointID, localauth.GetLocalListVersionFeatureName).Errorf("error on request: %v", err)
	//	} else {
	//		logDefault(chargePointID, confirmation.GetFeatureName()).Infof("current local list version: %v", confirmation.ListVersion)
	//	}
	//}
	//e = centralSystem.GetLocalListVersion(chargePointID, cb3)
	//if e != nil {
	//	logDefault(chargePointID, localauth.GetLocalListVersionFeatureName).Errorf("couldn't send message: %v", e)
	//	return
	//}
	// Wait for some time
	time.Sleep(5 * time.Second)
	configKey := "MeterValueSampleInterval"
	configValue := "10"
	// Change meter sampling values time
	cb4 := func(confirmation *core.ChangeConfigurationConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, core.ChangeConfigurationFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == core.ConfigurationStatusNotSupported {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for unsupported key: %v", configKey)
		} else if confirmation.Status == core.ConfigurationStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Warnf("couldn't update configuration for readonly key: %v", configKey)
		} else {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("updated configuration for key %v to: %v", configKey, configValue)
		}
	}
	e = centralSystem.ChangeConfiguration(chargePointID, cb4, configKey, configValue)
	if e != nil {
		logDefault(chargePointID, localauth.GetLocalListVersionFeatureName).Errorf("couldn't send message: %v", e)
		return
	}

	// Wait for some time
	time.Sleep(5 * time.Second)
	// Trigger a heartbeat message
	cb5 := func(confirmation *remotetrigger.TriggerMessageConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v triggered successfully", core.HeartbeatFeatureName)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v trigger was rejected", core.HeartbeatFeatureName)
		}
	}
	e = centralSystem.TriggerMessage(chargePointID, cb5, core.HeartbeatFeatureName)
	if e != nil {
		logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("couldn't send message: %v", e)
		return
	}

	// Wait for some time
	time.Sleep(5 * time.Second)
	// Trigger a diagnostics status notification
	cb6 := func(confirmation *remotetrigger.TriggerMessageConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v triggered successfully", firmware.GetDiagnosticsFeatureName)
		} else if confirmation.Status == remotetrigger.TriggerMessageStatusRejected {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("%v trigger was rejected", firmware.GetDiagnosticsFeatureName)
		}
	}
	e = centralSystem.TriggerMessage(chargePointID, cb6, firmware.DiagnosticsStatusNotificationFeatureName)
	if e != nil {
		logDefault(chargePointID, remotetrigger.TriggerMessageFeatureName).Errorf("couldn't send message: %v", e)
		return
	}
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
		handler.chargePoints[chargePoint.ID()] = &ChargePointState{connectors: map[int]*ConnectorInfo{}, transactions: map[int]*TransactionInfo{}}
		log.WithField("client", chargePoint.ID()).Info("new charge point connected")
		go setupRoutine(chargePoint.ID(), handler)
	})
	centralSystem.SetChargePointDisconnectedHandler(func(chargePoint ocpp16.ChargePointConnection) {
		log.WithField("client", chargePoint.ID()).Info("charge point disconnected")
		delete(handler.chargePoints, chargePoint.ID())
	})
	ocppj.SetLogger(log.WithField("logger", "ocppj"))
	//ws.SetLogger(log.WithField("logger", "websocket"))

	// Run central system
	log.Infof("starting central system on port %v", listenPort)
	centralSystem.Start(listenPort, "/{ws}")
	log.Info("stopped central system")
}

func init() {
	log = logrus.New()
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	// Set this to DebugLevel if you want to retrieve verbose logs from the ocppj and websocket layers
	log.SetLevel(logrus.InfoLevel)
}
