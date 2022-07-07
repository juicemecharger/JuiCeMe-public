package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/remotetrigger"
	"github.com/sirupsen/logrus"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/firmware"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

type Group struct {
	Chargers               map[string]string `json:"chargers"`
	DLMActionPending       bool              `json:"dlm_action_pending"`
	DLMLockedOut           bool              `json:"dlm_locked_out"` //Require manual unlocking in case of unexpected state
	MaxL1                  int               `json:"max_l1"`
	MaxL2                  int               `json:"max_l2"`
	MaxL3                  int               `json:"max_l3"`
	CurrentL1              int               `json:"current_l1"`
	CurrentL2              int               `json:"current_l2"`
	CurrentL3              int               `json:"current_l3"`
	AssignedL1             int               `json:"assigned_l1"`
	AssignedL2             int               `json:"assigned_l2"`
	AssignedL3             int               `json:"assigned_l3"`
	Offered3Phase          int               `json:"offered_3phase"`
	Initialized            bool              `json:"initialized"`
	AvarageAssignedCurrent int               `json:"avarage_assigned_current"`
}

// TransactionInfo contains info about a transaction
type TransactionInfo struct {
	Id          int             `json:"id"`
	StartTime   *types.DateTime `json:"start_time"`
	EndTime     *types.DateTime `json:"end_time"`
	StartMeter  int             `json:"start_meter"`
	EndMeter    int             `json:"end_meter"`
	ConnectorId int             `json:"connector_id"`
	IdTag       string          `json:"id_tag"`
}

func (ti *TransactionInfo) hasTransactionEnded() bool {
	return ti.EndTime != nil && !ti.EndTime.IsZero()
}

// ConnectorInfo contains status and ongoing transaction ID for a connector
type ConnectorInfo struct {
	Status             core.ChargePointStatus `json:"status"`
	Info               string                 `json:"info"`
	UnlockProgress     string                 `json:"unlock_progress"`
	CurrentTransaction int                    `json:"current_transaction"`
	DoneCharging       bool                   `json:"done_charging"`
	OnlyStandby        bool                   `json:"only_standby"`
}

type PortCurrents struct {
	L1 int `json:"l1"`
	L2 int `json:"l2"`
	L3 int `json:"l3"`
}

type PortPower struct {
	L1    int `json:"l1"`
	L2    int `json:"l2"`
	L3    int `json:"l3"`
	Total int `json:"total"`
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
	EVSEForDLMCycles            int                    `json:"evse_for_dlm_cycles"`
	OfflineForDLMCycles         int                    `json:"offline_for_dlm_cycles"`
	ReducedPowerOfferring       bool                   `json:"reduced_power_offerring"`
	MaxingPowerForDLMCycles     int                    `json:"maxing_power_for_dlm_cycles"`
	NotUsingMaxForDLMCycles     int                    `json:"not_using_max_for_dlm_cycles"`
	UsingLessThan6AForDLMCycles int                    `json:"using_less_than_6a_for_dlm_cycles"`
	Rotation                    string                 `json:"rotation"`
	Status                      core.ChargePointStatus `json:"status"`
	diagnosticsStatus           firmware.DiagnosticsStatus
	firmwareStatus              firmware.FirmwareStatus
	DLMGroup                    string                 `json:"dlm_group"`
	Connectors                  map[int]*ConnectorInfo `json:"connectors"`
	Currents                    PortCurrents           `json:"currents"`
	CurrentAssigned             PortCurrents           `json:"current_assigned"`
	CurrentTargeted             PortCurrents           `json:"current_targeted"`
	CurrentOffered              int                    `json:"current_offered"`
	Power                       PortPower              `json:"power"`
	EnergyMeterCurrent          int64                  `json:"energy_meter_current"`
	lastTimeStamp               *types.DateTime
	ErrorCode                   core.ChargePointErrorCode `json:"error_code"`
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
	ChargePoints            map[string]*ChargePointState `json:"charge_points"`
	Groups                  map[string]*Group            `json:"groups"`
	CurrentTotalL1          int                          `json:"current_total_l_1"`
	CurrentTotalL2          int                          `json:"current_total_l_2"`
	CurrentTotalL3          int                          `json:"current_total_l_3"`
	GroupsInitialized       map[string]bool              `json:"groups_initialized"`
	ChargePointsInitialized map[string]bool              `json:"charge_points_initialized"`
	Transactions            map[int]*TransactionInfo     `json:"transactions"`
	version                 string
	NextTransactionID       int `json:"next_transaction_id"`
	debug                   bool
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
			} else {
				authorized = types.AuthorizationStatusExpired
				logDefault(chargePointId, request.GetFeatureName()).Infof("card blocked")
			}
		} else {
			authorized = types.AuthorizationStatusBlocked
			logDefault(chargePointId, request.GetFeatureName()).Infof("card not in list")
		}

	}
	go handler.ResetDLM(chargePointId)
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
	if debugHearthBeat {
		logDefault(chargePointId, request.GetFeatureName()).Infof("heartbeat handled")
	}
	return core.NewHeartbeatConfirmation(types.NewDateTime(time.Now())), nil
}

func (handler *CentralSystemHandler) OnMeterValues(chargePointId string, request *core.MeterValuesRequest) (confirmation *core.MeterValuesConfirmation, err error) {
	if handler.debug {
		logDefault(chargePointId, request.GetFeatureName()).Infof("received meter values for connector %v. Meter values:\n", request.ConnectorId)
	}
	for _, mv := range request.MeterValue { //expect that only one meterValue per request is sent
		if handler.debug {
			logDefault(chargePointId, request.GetFeatureName()).Printf("%v", mv)
		}
		for _, sv := range mv.SampledValue {
			if handler.debug {
				log.Println("---------------------------")
				log.Println("SVV--" + sv.Value)
				log.Println("SVF--" + sv.Format)
				log.Println("SVC--" + sv.Context)
				log.Println("SVL--" + sv.Location)
				log.Println("SVP--" + sv.Phase)
				log.Println("SVM--" + sv.Measurand)
				log.Println("SVU--" + sv.Unit)
				log.Println("---------------------------")
			}
			switch sv.Measurand {
			case "Power.Active.Import":
				switch sv.Phase {
				case "L1":
					handler.ChargePoints[chargePointId].Power.L1, _ = strconv.Atoi(sv.Value)
				case "L2":
					handler.ChargePoints[chargePointId].Power.L2, _ = strconv.Atoi(sv.Value)
				case "L3":
					handler.ChargePoints[chargePointId].Power.L3, _ = strconv.Atoi(sv.Value)
				default:
					handler.ChargePoints[chargePointId].Power.Total, _ = strconv.Atoi(sv.Value)
				}
			case "Current.Offered":
				handler.ChargePoints[chargePointId].CurrentOffered, _ = strconv.Atoi(sv.Value)
			case "Current.Import":
				switch sv.Phase {
				case "L1":
					handler.ChargePoints[chargePointId].Currents.L1, _ = strconv.Atoi(sv.Value)
				case "L2":
					handler.ChargePoints[chargePointId].Currents.L2, _ = strconv.Atoi(sv.Value)
				case "L3":
					handler.ChargePoints[chargePointId].Currents.L3, _ = strconv.Atoi(sv.Value)
				default:
					log.Printf("Unexpected meterValue from %v", chargePointId)
				}
			case "Energy.Active.Import.Register":
				handler.ChargePoints[chargePointId].EnergyMeterCurrent, _ = strconv.ParseInt(sv.Value, 10, 64)
			}

		}
	}
	return core.NewMeterValuesConfirmation(), nil
}

func (handler *CentralSystemHandler) OnStatusNotification(chargePointId string, request *core.StatusNotificationRequest) (confirmation *core.StatusNotificationConfirmation, err error) {
	info, ok := handler.ChargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.ErrorCode = request.ErrorCode
	if request.ConnectorId > 0 {
		connectorInfo := info.getConnector(request.ConnectorId)
		connectorInfo.Status = request.Status
		connectorInfo.Info = request.Info
		if request.Status == "SuspendedEV" {
			connectorInfo.DoneCharging = true
			connectorInfo.OnlyStandby = true

		} else if request.Status == "SuspendedEVSE" && connectorInfo.DoneCharging {
			connectorInfo.DoneCharging = false
		} else if request.Status == "Charging" && request.Info == "Energy is flowing to vehicle" {
			connectorInfo.DoneCharging = false
		}
		logDefault(chargePointId, request.GetFeatureName()).Infof("connector %v updated status to %v", request.ConnectorId, request.Status)
		log.Println(request.Info)
	} else {
		info.Status = request.Status
		logDefault(chargePointId, request.GetFeatureName()).Infof("all connectors updated status to %v", request.Status)
	}
	return core.NewStatusNotificationConfirmation(), nil
}

func (handler *CentralSystemHandler) OnStartTransaction(chargePointId string, request *core.StartTransactionRequest) (confirmation *core.StartTransactionConfirmation, err error) {
	info, ok := handler.ChargePoints[chargePointId]
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
	transaction.Id = handler.NextTransactionID
	handler.NextTransactionID += 1
	connector.CurrentTransaction = transaction.Id
	handler.Transactions[transaction.Id] = transaction
	//Authorization-Check
	isMac := false
	idwithoutMac := strings.Replace(request.IdTag, "MAC", "", -1)
	log.Printf("ID_TAG: " + idwithoutMac)
	if (string([]rune(request.IdTag)[0]) == "M") && (string([]rune(request.IdTag)[1]) == "A") && (string([]rune(request.IdTag)[2]) == "C") {
		isMac = true
	}

	if isMac {
		_, exists := identity.MACs[idwithoutMac]
		if exists {
			logDefault(chargePointId, request.GetFeatureName()).Infof("transaction mac authorized")
		} else {
			logDefault(chargePointId, request.GetFeatureName()).Infof("mac not in list")
		}

	} else {
		_, exists := identity.Cards[request.IdTag]
		if exists {
			if identity.Cards[request.IdTag].Authorized {
				logDefault(chargePointId, request.GetFeatureName()).Infof("transaction card authorized")
			}
		} else {
			logDefault(chargePointId, request.GetFeatureName()).Infof("card not in list")
		}

	}

	//
	logDefault(chargePointId, request.GetFeatureName()).Infof("started transaction %v for connector %v", transaction.Id, transaction.ConnectorId)
	handler.ChargePoints[chargePointId].Connectors[1].OnlyStandby = true
	handler.ChargePoints[chargePointId].Connectors[1].DoneCharging = false
	return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted), transaction.Id), nil
}

func (handler *CentralSystemHandler) OnStopTransaction(chargePointId string, request *core.StopTransactionRequest) (confirmation *core.StopTransactionConfirmation, err error) {
	info, ok := handler.ChargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	transaction, ok := handler.Transactions[request.TransactionId]
	if ok {
		connector := info.getConnector(transaction.ConnectorId)
		connector.CurrentTransaction = -1
		transaction.EndTime = request.Timestamp
		transaction.EndMeter = request.MeterStop
		energyUsed := transaction.EndMeter - transaction.StartMeter
		//Detect if idtag is mac or card
		isMac := false
		idwithoutMac := strings.Replace(request.IdTag, "MAC", "", -1)
		log.Printf("ID_TAG: " + idwithoutMac)
		if (string([]rune(request.IdTag)[0]) == "M") && (string([]rune(request.IdTag)[1]) == "A") && (string([]rune(request.IdTag)[2]) == "C") {
			isMac = true
		}
		// isMac is true if it is a mac, idwithoutMac if just some form of id is needed
		//writing used energy onto id
		var tagident authIdStruct
		if isMac { //billing to mac
			tagident = identity.MACs[idwithoutMac]
		} else { //billing to normal id-tag
			tagident = identity.Cards[request.IdTag]
		}
		tagident.EnergyCharged += int64(energyUsed)
	}
	logDefault(chargePointId, request.GetFeatureName()).Infof("stopped transaction %v - %v", request.TransactionId, request.Reason)
	for _, mv := range request.TransactionData {
		logDefault(chargePointId, request.GetFeatureName()).Printf("%v", mv)
	}
	handler.ChargePoints[chargePointId].Connectors[1].OnlyStandby = false
	handler.ChargePoints[chargePointId].Connectors[1].DoneCharging = true
	return core.NewStopTransactionConfirmation(), nil
}

// ------------- Firmware management profile callbacks -------------

func (handler *CentralSystemHandler) OnDiagnosticsStatusNotification(chargePointId string, request *firmware.DiagnosticsStatusNotificationRequest) (confirmation *firmware.DiagnosticsStatusNotificationConfirmation, err error) {
	info, ok := handler.ChargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.diagnosticsStatus = request.Status
	logDefault(chargePointId, request.GetFeatureName()).Infof("updated diagnostics status to %v", request.Status)
	return firmware.NewDiagnosticsStatusNotificationConfirmation(), nil
}

func (handler *CentralSystemHandler) OnFirmwareStatusNotification(chargePointId string, request *firmware.FirmwareStatusNotificationRequest) (confirmation *firmware.FirmwareStatusNotificationConfirmation, err error) {
	info, ok := handler.ChargePoints[chargePointId]
	if !ok {
		return nil, fmt.Errorf("unknown charge point %v", chargePointId)
	}
	info.firmwareStatus = request.Status
	logDefault(chargePointId, request.GetFeatureName()).Infof("updated firmware status to %v", request.Status)
	return &firmware.FirmwareStatusNotificationConfirmation{}, nil
}

// No callbacks for Local Auth management, Reservation, Remote trigger or Smart Charging profile on central system

// Utility functions

// GetChargePointList Http-RPC
func (handler *CentralSystemHandler) GetChargePointList() map[string]*ChargePointState {
	return handler.ChargePoints
}

func (handler *CentralSystemHandler) GetSystemState() map[string]interface{} {
	reply := make(map[string]interface{})
	reply["chargePoints"] = handler.ChargePoints
	reply["groups"] = handler.Groups
	reply["identities"] = identity
	reply["debug"] = handler
	return reply
}

func (handler *CentralSystemHandler) SetChargePointRemoteStart(chargePointID string, idtag string) bool {
	println(chargePointID)
	callback3 := func(confirmation *core.RemoteStartTransactionConfirmation, err error) {
		log.Println("Confirmation")
	}
	_ = centralSystem.RemoteStartTransaction(chargePointID, callback3, idtag)
	return true
}

func (handler *CentralSystemHandler) SetChargePointRemoteStop(chargePointID string) bool {
	println(chargePointID)
	callback3 := func(confirmation *core.RemoteStopTransactionConfirmation, err error) {
		log.Println("Confirmation")
	}
	txid := handler.ChargePoints[chargePointID].Connectors[1].CurrentTransaction
	println(txid)
	_ = centralSystem.RemoteStopTransaction(chargePointID, callback3, txid)
	return true
}

func (handler *CentralSystemHandler) UnlockPort(chargePointID string, ConnID int) {
	handler.ChargePoints[chargePointID].Connectors[ConnID].UnlockProgress = ""
	callback4 := func(confirm *core.UnlockConnectorConfirmation, err error) {
		handler.ChargePoints[chargePointID].Connectors[ConnID].UnlockProgress = string(confirm.Status)
	}
	_ = centralSystem.UnlockConnector(chargePointID, callback4, ConnID) //Always 1 one JuiceME Chargers, but we just define one in case Param not given (in server.go)
	return
}

func (handler *CentralSystemHandler) OverridePowerTarget(chargePointID string, limit string) bool {
	_, exists := handler.ChargePoints[chargePointID]
	var err error
	if exists {
		handler.ChargePoints[chargePointID].CurrentTargeted.L1, err = strconv.Atoi(limit)
		handler.ChargePoints[chargePointID].CurrentTargeted.L2, err = strconv.Atoi(limit)
		handler.ChargePoints[chargePointID].CurrentTargeted.L3, err = strconv.Atoi(limit)
		if err == nil {
			groupid := handler.ChargePoints[chargePointID].DLMGroup
			handler.Groups[groupid].DLMActionPending = true
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}

//END http-rpc

func logDefault(chargePointId string, feature string) *logrus.Entry {
	return log.WithFields(logrus.Fields{"client": chargePointId, "message": feature})
}

func (handler *CentralSystemHandler) chargePointByID(id string) (*ChargePointState, error) {
	cp, ok := handler.ChargePoints[id]
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
