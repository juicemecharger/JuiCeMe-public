package main

import (
	"strconv"
	"time"
)

func MustParseDuration(s string) time.Duration {
	value, err := time.ParseDuration(s)
	if err != nil {
		panic("util: Can't parse duration `" + s + "`: " + err.Error())
	}
	return value
}

func (handler *CentralSystemHandler) dlmstart() {
	log.Println("Starting DLM")
	interval := MustParseDuration("10s")
	timer := time.NewTimer(interval)
	timer.Reset(interval)
	go func() {
		for {
			select {
			case <-timer.C:
				handler.dlm()
				timer.Reset(interval)
			}
		}
	}()
}

func (handler *CentralSystemHandler) isOKChangingCurrent(chargePointID string, wanted PortCurrents) {

}

func (handler *CentralSystemHandler) dlm() {
	for name, cp := range handler.ChargePoints {
		if cp.CurrentAssigned != cp.CurrentTargeted {
			success := handler.SetConfig(name, "DlmOperatorPhase1Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L1))
			if success {
				success = handler.SetConfig(name, "DlmOperatorPhase2Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L2))
			}
			if success {
				success = handler.SetConfig(name, "DlmOperatorPhase3Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L3))
			}
			if !success {
				log.Println("Error whilst setting current!!!!!!!!!!!!!!!!!!!!!")
			} else {
				cp.CurrentAssigned = cp.CurrentTargeted
			}
		}
	}
}

func (handler *CentralSystemHandler) AssignPowerOnAuth(chargePointID string) {

}
