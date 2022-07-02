package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/sirupsen/logrus"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

var (
	nextTransactionId = 0
)

type Group struct {
	Chargers    map[string]string
	MaxL1       uint
	MaxL2       uint
	MaxL3       uint
	CurrentL1   uint
	CurrentL2   uint
	CurrentL3   uint
	Initialized bool
}

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

//type CardStruct struct {
//	Authorized   string
//	Transactions []TransactionInfo
//}

// ChargePointState contains all relevant state data for a connected charge point, simplified only working with single-connector chargepoints
type ChargePointState struct {
	Status            core.ChargePointStatus
	diagnosticsStatus firmware.DiagnosticsStatus
	firmwareStatus    firmware.FirmwareStatus
	Connectors        map[int]*ConnectorInfo // No assumptions about the # of connectors && In case of Bender / JuiceME , #1 is the only connector
	Currents          PortCurrents
	CurrentAssigned   PortCurrents
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

// CentralSystemHandler contains  state that central system wants to keep.
type CentralSystemHandler struct {
	chargePoints   map[string]*ChargePointState
	groups         map[string]*Group
	CurrentTotalL1 uint
	CurrentTotalL2 uint
	CurrentTotalL3 uint
	version        string
}

// ------------- Core profile callbacks -------------

func (handler *CentralSystemHandler) OnAuthorize(chargePointId string, request *core.AuthorizeRequest) (confirmation *core.AuthorizeConfirmation, err error) {
	var authorized types.AuthorizationStatus
	isMac := false
	idwithoutMac := strings.Replace(request.IdTag, "MAC", "", -1)
	log.Printf("ID_TAG: " + idwithoutMac)
	if (string([]rune(request.IdTag)[0]) == "M") && (string([]rune(request.IdTag)[1]) == "A") && (string([]rune(request.IdTag)[2]) == "C") {
		isMac = true
	}

	if isMac {
		_, exists := identity.MACs[idwithoutMac]
		if exists {
			if identity.MACs[idwithoutMac].Authorized {
				authorized = types.AuthorizationStatusAccepted
				logDefault(chargePointId, request.GetFeatureName()).Infof("mac authorized")
				go handler.AssignPowerOnAuth(chargePointId)
			} else {
				authorized = types.AuthorizationStatusExpired
				logDefault(chargePointId, request.GetFeatureName()).Infof("mac blocked")
			}
		} else {
			authorized = types.AuthorizationStatusBlocked
			logDefault(chargePointId, request.GetFeatureName()).Infof("mac not in list")
		}

	} else {
		_, exists := identity.Cards[request.IdTag]
		if exists {
			if identity.Cards[request.IdTag].Authorized {
				authorized = types.AuthorizationStatusAccepted
				logDefault(chargePointId, request.GetFeatureName()).Infof("card authorized")
				go handler.AssignPowerOnAuth(chargePointId)
			} else {
				authorized = types.AuthorizationStatusExpired
				logDefault(chargePointId, request.GetFeatureName()).Infof("card blocked")
			}
		} else {
			authorized = types.AuthorizationStatusBlocked
			logDefault(chargePointId, request.GetFeatureName()).Infof("card not in list")
		}

	}

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

func (handler *CentralSystemHandler) GetSystemState() map[string]interface{} {
	reply := make(map[string]interface{})
	reply["chargePoints"] = handler.chargePoints
	reply["groups"] = handler.groups
	reply["identities"] = identity
	return reply
}

func (handler *CentralSystemHandler) SetChargePointStart(chargePointID string) bool {
	//success := true
	println(chargePointID)
	//callback3 := func(confirmation *core.RemoteStartTransactionConfirmation, err error) {
	//	log.Println("Confirmation")
	//}
	//centralSystem.
	return true
}

func (handler *CentralSystemHandler) UnlockPort(chargePointID string, ConnID int) {
	handler.chargePoints[chargePointID].Connectors[ConnID].UnlockProgress = ""
	callback4 := func(confirm *core.UnlockConnectorConfirmation, err error) {
		handler.chargePoints[chargePointID].Connectors[ConnID].UnlockProgress = string(confirm.Status)
	}
	_ = centralSystem.UnlockConnector(chargePointID, callback4, ConnID) //Always 1 one JuiceME Chargers, but we just define one in case Param not given (in server.go)
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
	_ = centralSystem.ChangeConfiguration(id, callback5, key, value)
	for softfail < 10 {
		softfail++
		if success == true {
			return success
		}
		time.Sleep(500 * time.Millisecond)
	}
	return success
}
