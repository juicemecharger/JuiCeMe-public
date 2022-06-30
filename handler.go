package main

import (
	"fmt"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/sirupsen/logrus"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

var (
	nextTransactionId = 0
	//modclient         *modbus.ModbusClient //because bmw we dont
)

// TransactionInfo contains info about a transaction
type TransactionInfo struct {
	Id          int
	StartTime   *types.DateTime
	EndTime     *types.DateTime
	StartMeter  int
	EndMeter    int
	ConnectorId int
	IdTag       string
}

func (ti *TransactionInfo) hasTransactionEnded() bool {
	return ti.EndTime != nil && !ti.EndTime.IsZero()
}

// ConnectorInfo contains status and ongoing transaction ID for a connector
type ConnectorInfo struct {
	Status             core.ChargePointStatus
	UnlockProgress     string
	CurrentTransaction int
}

type PortCurrents struct {
	L1 uint
	L2 uint
	L3 uint
}

type PortPower struct {
	L1    uint
	L2    uint
	L3    uint
	Total uint
}

func (ci *ConnectorInfo) hasTransactionInProgress() bool {
	return ci.CurrentTransaction >= 0
}

type jsondata struct {
	Data map[string]cardstruct
}

type cardstruct struct {
	Authorized   string
	Transactions []TransactionInfo
}

// ChargePointState contains all relevant state data for a connected charge point, simplified only working with single-connector chargepoints
type ChargePointState struct {
	Status            core.ChargePointStatus
	diagnosticsStatus firmware.DiagnosticsStatus
	firmwareStatus    firmware.FirmwareStatus
	Connectors        map[int]*ConnectorInfo // No assumptions about the # of connectors && In case of Bender / JuiceME , #1 is the only connector
	Currents          PortCurrents
	Power             PortPower
	lastTimeStamp     *types.DateTime
	Transactions      map[int]*TransactionInfo
	ErrorCode         core.ChargePointErrorCode
}

func (cps *ChargePointState) getConnector(id int) *ConnectorInfo {
	ci, ok := cps.Connectors[id]
	if !ok {
		ci = &ConnectorInfo{CurrentTransaction: -1}
		cps.Connectors[id] = ci
	}
	return ci
}

// CentralSystemHandler contains some simple state that a central system may want to keep.
// In production this will typically be replaced by database/API calls.
type CentralSystemHandler struct {
	chargePoints map[string]*ChargePointState
	version      string
}

// ------------- Core profile callbacks -------------

func (handler *CentralSystemHandler) OnAuthorize(chargePointId string, request *core.AuthorizeRequest) (confirmation *core.AuthorizeConfirmation, err error) {
	var authorized types.AuthorizationStatus
	log.Printf("ID_TAG: " + request.IdTag)
	if request.IdTag == "113e0236" || request.IdTag == "40b66b7c" || request.IdTag == "bd52b9a0" || request.IdTag == "DC442718546C" {
		authorized = types.AuthorizationStatusAccepted
	} else {
		authorized = types.AuthorizationStatusBlocked
	}
	logDefault(chargePointId, request.GetFeatureName()).Infof("client authorized")
	return core.NewAuthorizationConfirmation(types.NewIdTagInfo(authorized)), nil
}

func (handler *CentralSystemHandler) OnBootNotification(chargePointId string, request *core.BootNotificationRequest) (confirmation *core.BootNotificationConfirmation, err error) {
	logDefault(chargePointId, request.GetFeatureName()).Infof("boot confirmed")
	return core.NewBootNotificationConfirmation(types.NewDateTime(time.Now()), defaultHeartbeatInterval, core.RegistrationStatusAccepted), nil
}

func (handler *CentralSystemHandler) OnDataTransfer(chargePointId string, request *core.DataTransferRequest) (confirmation *core.DataTransferConfirmation, err error) {
	logDefault(chargePointId, request.GetFeatureName()).Infof("received data %d", request.Data)
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (handler *CentralSystemHandler) OnHeartbeat(chargePointId string, request *core.HeartbeatRequest) (confirmation *core.HeartbeatConfirmation, err error) {
	logDefault(chargePointId, request.GetFeatureName()).Infof("heartbeat handled")
	return core.NewHeartbeatConfirmation(types.NewDateTime(time.Now())), nil
}

func (handler *CentralSystemHandler) OnMeterValues(chargePointId string, request *core.MeterValuesRequest) (confirmation *core.MeterValuesConfirmation, err error) {
	logDefault(chargePointId, request.GetFeatureName()).Infof("received meter values for connector %v. Meter values:\n", request.ConnectorId)
	for _, mv := range request.MeterValue {
		logDefault(chargePointId, request.GetFeatureName()).Printf("%v", mv)
	}
	return core.NewMeterValuesConfirmation(), nil
}

func (handler *CentralSystemHandler) OnStatusNotification(chargePointId string, request *core.StatusNotificationRequest) (confirmation *core.StatusNotificationConfirmation, err error) {
	info, ok := handler.chargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.ErrorCode = request.ErrorCode
	if request.ConnectorId > 0 {
		connectorInfo := info.getConnector(request.ConnectorId)
		connectorInfo.Status = request.Status
		//if connectorInfo.status == "Charging" {
		//	time.Sleep(1 * time.Second)
		// EV is plugged in
		//modclient, err = modbus.NewClient(&modbus.ClientConfiguration{
		//	URL:     "tcp://" + info.ipadress + ":502",
		//	Timeout: 1 * time.Second,
		//})
		//Modbus Request EVCC_ID
		//Test- EVCCID

		//ENDE TEST

		//} else if connectorInfo.status == "Available" {
		//	//No EV is plugged in
		//	connectorInfo.evccid = "None"
		//}
		logDefault(chargePointId, request.GetFeatureName()).Infof("connector %v updated status to %v", request.ConnectorId, request.Status)
		log.Println(request.Info)
	} else {
		info.Status = request.Status
		logDefault(chargePointId, request.GetFeatureName()).Infof("all connectors updated status to %v", request.Status)
	}
	return core.NewStatusNotificationConfirmation(), nil
}

func (handler *CentralSystemHandler) OnStartTransaction(chargePointId string, request *core.StartTransactionRequest) (confirmation *core.StartTransactionConfirmation, err error) {
	info, ok := handler.chargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	connector := info.getConnector(request.ConnectorId)
	if connector.CurrentTransaction >= 0 {
		return nil, fmt.Errorf("connector %v is currently busy with another transaction", request.ConnectorId)
	}
	transaction := &TransactionInfo{}
	transaction.IdTag = request.IdTag
	transaction.ConnectorId = request.ConnectorId
	transaction.StartMeter = request.MeterStart
	transaction.StartTime = request.Timestamp
	transaction.Id = nextTransactionId
	nextTransactionId += 1
	connector.CurrentTransaction = transaction.Id
	info.Transactions[transaction.Id] = transaction
	//TODO: check billable clients
	logDefault(chargePointId, request.GetFeatureName()).Infof("started transaction %v for connector %v", transaction.Id, transaction.ConnectorId)
	return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted), transaction.Id), nil
}

func (handler *CentralSystemHandler) OnStopTransaction(chargePointId string, request *core.StopTransactionRequest) (confirmation *core.StopTransactionConfirmation, err error) {
	info, ok := handler.chargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	transaction, ok := info.Transactions[request.TransactionId]
	if ok {
		connector := info.getConnector(transaction.ConnectorId)
		connector.CurrentTransaction = -1
		transaction.EndTime = request.Timestamp
		transaction.EndMeter = request.MeterStop
		//TODO: bill charging period to client
	}
	logDefault(chargePointId, request.GetFeatureName()).Infof("stopped transaction %v - %v", request.TransactionId, request.Reason)
	for _, mv := range request.TransactionData {
		logDefault(chargePointId, request.GetFeatureName()).Printf("%v", mv)
	}
	return core.NewStopTransactionConfirmation(), nil
}

// ------------- Firmware management profile callbacks -------------

func (handler *CentralSystemHandler) OnDiagnosticsStatusNotification(chargePointId string, request *firmware.DiagnosticsStatusNotificationRequest) (confirmation *firmware.DiagnosticsStatusNotificationConfirmation, err error) {
	info, ok := handler.chargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.diagnosticsStatus = request.Status
	logDefault(chargePointId, request.GetFeatureName()).Infof("updated diagnostics status to %v", request.Status)
	return firmware.NewDiagnosticsStatusNotificationConfirmation(), nil
}

func (handler *CentralSystemHandler) OnFirmwareStatusNotification(chargePointId string, request *firmware.FirmwareStatusNotificationRequest) (confirmation *firmware.FirmwareStatusNotificationConfirmation, err error) {
	info, ok := handler.chargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.firmwareStatus = request.Status
	logDefault(chargePointId, request.GetFeatureName()).Infof("updated firmware status to %v", request.Status)
	return &firmware.FirmwareStatusNotificationConfirmation{}, nil
}

// No callbacks for Local Auth management, Reservation, Remote trigger or Smart Charging profile on central system

// Utility functions

func (handler *CentralSystemHandler) GetChargePointList() map[string]*ChargePointState {
	return handler.chargePoints
}

func (handler *CentralSystemHandler) SetChargePointStart(chargePointID string) bool {
	success := true
	//callback3 := func(confirmation *core.RemoteStartTransactionConfirmation, err error) {
	//	log.Println("Confirmation")
	//}
	//centralSystem.
	return success
}

func (handler *CentralSystemHandler) UnlockPort(chargePointID string, ConnID int) {
	handler.chargePoints[chargePointID].Connectors[ConnID].UnlockProgress = ""
	callback4 := func(confirm *core.UnlockConnectorConfirmation, err error) {
		handler.chargePoints[chargePointID].Connectors[ConnID].UnlockProgress = string(confirm.Status)
	}
	centralSystem.UnlockConnector(chargePointID, callback4, ConnID) //Always 1 one JuiceME Chargers, but we just define one in case Param not given (in server.go)
	return
}

func logDefault(chargePointId string, feature string) *logrus.Entry {
	return log.WithFields(logrus.Fields{"client": chargePointId, "message": feature})
}

func (handler *CentralSystemHandler) chargePointByID(id string) (*ChargePointState, error) {
	cp, ok := handler.chargePoints[id]
	if !ok {
		return nil, fmt.Errorf("unknown charge point: %s", id)
	}
	return cp, nil
}

func (handler *CentralSystemHandler) SetConfig(id string, key string, value string) bool {
	var success = false
	log.Println(key)
	log.Println(value)
	softfail := 0
	callback5 := func(confirmation *core.ChangeConfigurationConfirmation, err error) {
		if err != nil {
			logDefault(id, remotetrigger.TriggerMessageFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == core.ConfigurationStatusAccepted {
			logDefault(id, confirmation.GetFeatureName()).Infof("%v triggered successfully", core.ChangeConfigurationFeatureName)
			success = true
		} else if confirmation.Status == core.ConfigurationStatusRejected || confirmation.Status == core.ConfigurationStatusNotSupported {
			logDefault(id, confirmation.GetFeatureName()).Infof("%v trigger was rejected", core.ChangeConfigurationFeatureName)
		}
	}
	centralSystem.ChangeConfiguration(id, callback5, key, value)
	for softfail < 10 {
		softfail++
		if success == true {
			return success
		}
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

func (handler *CentralSystemHandler) SinglePhaseForce(id string, connector int) bool {
	//centralSystem.RemoteStartTransaction()
	return false
}
