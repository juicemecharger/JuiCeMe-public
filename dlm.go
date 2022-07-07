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
	interval := MustParseDuration("1s")
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
	currentoffered := make(map[string]int)
	for name, _ := range handler.Groups {
		currentoffered[name] = 0
	}
	// Setting targeted Currents and ramping down only!!!
	for name, cp := range handler.ChargePoints {
		groupid := cp.DLMGroup
		if cp.CurrentAssigned != cp.CurrentTargeted {
			success1 := handler.SetConfig(name, "DlmOperatorPhase1Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L1))
			success2 := handler.SetConfig(name, "DlmOperatorPhase2Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L2))
			success3 := handler.SetConfig(name, "DlmOperatorPhase3Limit", strconv.Itoa(handler.ChargePoints[name].CurrentTargeted.L3))
			if !success1 || !success2 || !success3 {
				log.Println("Error whilst setting current from DLM")
			} else {
				cp.CurrentAssigned = cp.CurrentTargeted
				handler.Groups[groupid].DLMActionPending = false
			}
		}
		if len(cp.Connectors) == 1 {
			if cp.Connectors[1].Status == "Charging" && !cp.Connectors[1].DoneCharging {
				currentoffered[groupid] += cp.CurrentOffered
			}
			if cp.Connectors[1].Status == "Available" && cp.CurrentAssigned.L1 != 0 && !handler.Groups[groupid].DLMActionPending {
				cp.CurrentTargeted.L1 = 0
				cp.CurrentTargeted.L2 = 0
				cp.CurrentTargeted.L3 = 0
				handler.Groups[groupid].DLMActionPending = true
				log.Printf("Chargepoint %v done charging/unplugged, reducing current to 0", name)
			}
			if cp.Connectors[1].Status == "Unavailable" && cp.CurrentAssigned.L1 != 0 && cp.OfflineForDLMCycles > 70 {
				cp.CurrentTargeted.L1 = 0
				cp.CurrentTargeted.L2 = 0
				cp.CurrentTargeted.L3 = 0
				handler.Groups[groupid].DLMActionPending = true
				log.Printf("Chargepoint %v unavailable/offline, reducing current to 0", name)
			} else if cp.Connectors[1].Status == "Unavailable" {
				cp.OfflineForDLMCycles++
			} else {
				cp.OfflineForDLMCycles = 0
			}
		}
	}

	for name, currentofferedvalue := range currentoffered {
		handler.Groups[name].Offered3Phase = currentofferedvalue
	}
	handler.AssignedPowerGroupSumUp()
	handler.RampUpPower()

}

func (handler *CentralSystemHandler) AssignedPowerGroupSumUp() {
	handler.CurrentTotalL1 = 0
	handler.CurrentTotalL2 = 0
	handler.CurrentTotalL3 = 0
	assigned := make(map[string]map[string]int)

	for name, _ := range handler.Groups {
		assigned[name] = map[string]int{}
		//assignedcurrent
		assigned[name]["L1AA"] = 0
		assigned[name]["L2AA"] = 0
		assigned[name]["L3AA"] = 0
		//realtimecurrent
		assigned[name]["L3AC"] = 0
		assigned[name]["L3AC"] = 0
		assigned[name]["L3AC"] = 0

	}
	for _, cp := range handler.ChargePoints {
		groupid := cp.DLMGroup
		//assignedcurrents
		assigned[groupid]["L1AA"] += cp.CurrentAssigned.L1
		assigned[groupid]["L2AA"] += cp.CurrentAssigned.L2
		assigned[groupid]["L3AA"] += cp.CurrentAssigned.L3
		//currentpower
		assigned[groupid]["L1AC"] += cp.Currents.L1
		assigned[groupid]["L2AC"] += cp.Currents.L2
		assigned[groupid]["L3AC"] += cp.Currents.L3
	}
	for name, valuesmap := range assigned {
		//assignedcurrent
		handler.Groups[name].AssignedL1 = valuesmap["L1AA"]
		handler.Groups[name].AssignedL2 = valuesmap["L2AA"]
		handler.Groups[name].AssignedL3 = valuesmap["L3AA"]
		//currentpower
		handler.Groups[name].CurrentL1 = valuesmap["L1AC"]
		handler.Groups[name].CurrentL2 = valuesmap["L2AC"]
		handler.Groups[name].CurrentL3 = valuesmap["L3AC"]
		//globalpower
		handler.CurrentTotalL1 += valuesmap["L1AC"]
		handler.CurrentTotalL2 += valuesmap["L2AC"]
		handler.CurrentTotalL3 += valuesmap["L3AC"]
	}
	//availableforgroup

}

func (handler *CentralSystemHandler) RampUpPower() {
	wantsfullpower := make(map[string]map[string]bool)
	newpower := make(map[string]map[string]int)
	for name, _ := range handler.Groups {
		newpower[name] = map[string]int{}
		newpower[name]["group"] = 0
		wantsfullpower[name] = map[string]bool{}
	}
	currentleftover := make(map[string]int)
	for name, grp := range handler.Groups {
		currentleftover[name] = grp.MaxL1 - grp.AssignedL1
	}
	for name, cp := range handler.ChargePoints {
		if cp.Status == "Available" && len(cp.Connectors) == 1 { //Not shut down and Juice ME charger
			groupid := cp.DLMGroup
			if !cp.Connectors[1].DoneCharging {
				//Method for giving standby Power
				if cp.Connectors[1].OnlyStandby && !cp.Connectors[1].DoneCharging {
					cp.CurrentTargeted.L1 = 6
					cp.CurrentTargeted.L2 = 6
					cp.CurrentTargeted.L3 = 6
					grouppower := newpower[groupid]
					cp.ReducedPowerOfferring = true
					grouppower["group"] += 6
					handler.Groups[groupid].DLMActionPending = true
				}

				//Method for detecting Repower after standby has been detected
				if cp.Connectors[1].OnlyStandby && cp.MaxingPowerForDLMCycles > dlmrampupfromstandby {
					cp.Connectors[1].OnlyStandby = false
					handler.ResetDLM(name)
					log.Printf("%v wants more power, pulling them out of stanby 6A mode after they maxed that for %v times", name, dlmrampupfromstandby)
					//Car wants moar powaaarrr, assume that charging limit was increased, thus we assume it as normal charging at full rate
					//wantsfullpower[groupid][name] = true
					cp.CurrentTargeted.L1 = 8
					cp.CurrentTargeted.L2 = 8
					cp.CurrentTargeted.L3 = 8
					grouppower := newpower[groupid]
					cp.ReducedPowerOfferring = true
					grouppower["group"] += 8
				} else if cp.Connectors[1].OnlyStandby && (cp.Currents.L1 > dlmrampupfromstandbyaftercurrent || cp.Currents.L2 > dlmrampupfromstandbyaftercurrent || cp.Currents.L3 > dlmrampupfromstandbyaftercurrent) {
					cp.MaxingPowerForDLMCycles = cp.MaxingPowerForDLMCycles + 1
					log.Printf("%v maxing standby 6A, waiting for total %v/%v cycles for rampup", name, cp.MaxingPowerForDLMCycles, dlmrampupfromstandby)
				}
				if cp.Currents.L1 < 6 && cp.Currents.L2 < 6 && cp.Currents.L3 < 6 && !cp.Connectors[1].OnlyStandby {
					//Not in standby current mode, but using less than 6 Amps
					cp.UsingLessThan6AForDLMCycles++
					log.Printf("%v using less than 6A, putting them into stanby 6A mode after they continue that for %v times", name, timetostandbyvehicle)
					if cp.UsingLessThan6AForDLMCycles > timetostandbyvehicle {
						log.Printf("Putting %v into standby after only using less than 6 Amps", name)
						cp.Connectors[1].OnlyStandby = true //6A standby Current
					}
				}

				if cp.Currents.L1 < cp.CurrentAssigned.L1-dlmrampdownafterunusedcurrent && cp.Currents.L2 < cp.CurrentAssigned.L2-dlmrampdownafterunusedcurrent && cp.Currents.L3 < cp.CurrentAssigned.L3-dlmrampdownafterunusedcurrent && !cp.Connectors[1].OnlyStandby {
					//Car isn't using 100% of its assigned power from the station ( 2 less than assigned
					if cp.NotUsingMaxForDLMCycles > dlmrampdownafterunusedcurrentfor {
						if cp.Currents.L1+rampdowntocurrentoffset > 6 || cp.Currents.L2+rampdowntocurrentoffset > 6 || cp.Currents.L3+rampdowntocurrentoffset > 6 {
							//Car doesn't use maximum full Power and is not using less than or equal 5A
							log.Printf("%v has been ramped down to (%v/%v/%v)A", name, cp.Currents.L1+rampdowntocurrentoffset, cp.Currents.L2+rampdowntocurrentoffset, cp.Currents.L3+rampdowntocurrentoffset)
							cp.CurrentTargeted.L1 = cp.Currents.L1 + rampdowntocurrentoffset
							cp.CurrentTargeted.L2 = cp.Currents.L2 + rampdowntocurrentoffset
							cp.CurrentTargeted.L3 = cp.Currents.L3 + rampdowntocurrentoffset
							cp.ReducedPowerOfferring = true
							grouppower := newpower[groupid]
							grouppower["group"] += cp.CurrentTargeted.L1
							handler.ResetDLM(name)
						} else {
							//Car is using 5A or less
							cp.Connectors[1].OnlyStandby = true
							log.Printf("%v went to standbycurrent from 2nd function, thats unuaual......")
						}
					} else {
						cp.NotUsingMaxForDLMCycles++
						log.Printf("%v is not using maximum Power for %v/%v cycles, they will soon be ramped down", name, cp.NotUsingMaxForDLMCycles, dlmrampdownafterunusedcurrentfor)
					}
				} else {
					//Car uses Assigned power
					if cp.NotUsingMaxForDLMCycles > 0 {
						log.Printf("%v is over the threshold again, resetting counter")
					}
					cp.NotUsingMaxForDLMCycles = 0
				}

				if (cp.Currents.L1 == cp.CurrentOffered || cp.Currents.L2 == cp.CurrentOffered || cp.Currents.L3 == cp.CurrentOffered) && cp.ReducedPowerOfferring && !cp.Connectors[1].OnlyStandby {
					//Car using all of its assigned power, and power offering to station is reduced
					if cp.CurrentOffered == cp.CurrentTargeted.L1 || cp.CurrentOffered == cp.CurrentTargeted.L2 || cp.CurrentOffered == cp.CurrentTargeted.L3 {
						//Internal Dlm of station has ramped up to 100% of its assigned current
						if cp.MaxingPowerForDLMCycles >= dlmrampupfromstandby {
							//Car does use maximum full Power, time to recheck
							wantsfullpower[groupid][name] = true
							cp.ReducedPowerOfferring = false
							handler.ResetDLM(name)
						} else {
							cp.MaxingPowerForDLMCycles++
							log.Printf("%v is maxing assigned power and station assigned cap is maxed, %v/%v times left before rampup", name, cp.MaxingPowerForDLMCycles, dlmrampupfromstandby)
						}
					} else {
						//Power offering is reduced and car is using all of the stations assigned power, but station isnt offering all of its power yet

					}
				}
				//Normal (full load) DLM after here
				if !cp.Connectors[1].OnlyStandby && !cp.Connectors[1].DoneCharging && !cp.ReducedPowerOfferring {
					wantsfullpower[groupid][name] = true
				}
			} else {
				//Car only plugged in, but not using any power
				if cp.CurrentTargeted.L1 != 6 {
					log.Printf("%v stopped charging, removing power assignment", name)
					cp.CurrentTargeted.L1 = 6
					cp.CurrentTargeted.L2 = 6
					cp.CurrentTargeted.L3 = 6
				} else {
					if cp.Currents.L1 > 0 || cp.Currents.L2 > 0 || cp.Currents.L3 > 0 && cp.Connectors[1].DoneCharging {
						handler.SetConfig(name, "DlmOperatorPhase1Limit", "0")
						handler.SetConfig(name, "DlmOperatorPhase2Limit", "0")
						handler.SetConfig(name, "DlmOperatorPhase3Limit", "0")
						cp.CurrentAssigned.L1 = 0
						cp.CurrentTargeted.L1 = 0
						cp.CurrentAssigned.L2 = 0
						cp.CurrentTargeted.L2 = 0
						cp.CurrentAssigned.L3 = 0
						cp.CurrentTargeted.L3 = 0
					}
				}
			}
		}
		if debugHearthBeat {
			time.Sleep(2 * time.Millisecond)
		}
	}
	log.Println(wantsfullpower)
	log.Println("---------------------------------DLMCollectorEnd--------------------------------------")
	log.Println(" ")
	log.Println("---------------------------------DLMGroupAssignmentStart--------------------------------------")
	for groupid, chargermap := range wantsfullpower {
		acivechargers := 0
		groupavailablecurrent := handler.Groups[groupid].MaxL1
		grouppower := newpower[groupid]
		groupavailablecurrent = groupavailablecurrent - grouppower["group"]
		log.Printf("%v A available remaining after ReducedCurrentOfferings for distribution", groupavailablecurrent)
		for _, allthepower := range chargermap {
			if allthepower {
				acivechargers++
			}
		}
		if acivechargers != 0 {
			medianavailable := groupavailablecurrent / acivechargers
			if medianavailable > 16 {
				medianavailable = 16
			}
			if debugHearthBeat {
				log.Printf("------------- MaxPowerDLM - Group %v -------------------", groupid)
				log.Printf("  MedianPower: %v", medianavailable)

			}
			for name, _ := range chargermap {
				handler.ChargePoints[name].CurrentTargeted.L1 = medianavailable
				handler.ChargePoints[name].CurrentTargeted.L2 = medianavailable
				handler.ChargePoints[name].CurrentTargeted.L3 = medianavailable
				if debugHearthBeat {
					log.Printf("  Startion %v Power: %v", name, medianavailable)
				}
			}
		} else {
			log.Printf("------------- MaxPowerDLM - Group %v -------------------", groupid)
			log.Printf("_-_-_-_-_-_ SKIPPED, none want full power _-_-_-_-_-_-_-_-")
		}

	}
	log.Println("---------------------------------DLMGroupAssignmentEND--------------------------------------")
	return
}

func (handler *CentralSystemHandler) ResetDLM(chargePointID string) {
	log.Printf("Resetting DLM Counters for %v", chargePointID)
	handler.ChargePoints[chargePointID].MaxingPowerForDLMCycles = 0
	handler.ChargePoints[chargePointID].NotUsingMaxForDLMCycles = 0
	handler.ChargePoints[chargePointID].UsingLessThan6AForDLMCycles = 0
}
