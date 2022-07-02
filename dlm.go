package main

import "time"

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

				timer.Reset(interval)
			}
		}
	}()
}

func (handler *CentralSystemHandler) dlm() {

}

func (handler *CentralSystemHandler) AssignPowerOnAuth(chargePointID string) {

}
