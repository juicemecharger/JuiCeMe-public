JuiCeME-Central-System
----------------------


Reservation Example

    // Wait for some time
	time.Sleep(2 * time.Second)
	// Reserve a connector
	reservationID := 42
	clientIdTag := "test"
	connectorID := 1
	expiryDate := types.NewDateTime(time.Now().Add(1 * time.Hour))
	cb1 := func(confirmation *reservation.ReserveNowConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, reservation.ReserveNowFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == reservation.ReservationStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("connector %v reserved for client %v until %v (reservation ID %d)", connectorID, clientIdTag, expiryDate.FormatTimestamp(), reservationID)
		} else {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("couldn't reserve connector %v: %v", connectorID, confirmation.Status)
		}
	}
	e := centralSystem.ReserveNow(chargePointID, cb1, connectorID, expiryDate, clientIdTag, reservationID)
	if e != nil {
		logDefault(chargePointID, reservation.ReserveNowFeatureName).Errorf("couldn't send message: %v", e)
		return
	}
	// Wait for some time
	time.Sleep(1 * time.Second)
	// Cancel the reservation
	cb2 := func(confirmation *reservation.CancelReservationConfirmation, err error) {
		if err != nil {
			logDefault(chargePointID, reservation.CancelReservationFeatureName).Errorf("error on request: %v", err)
		} else if confirmation.Status == reservation.CancelReservationStatusAccepted {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("reservation %v canceled successfully", reservationID)
		} else {
			logDefault(chargePointID, confirmation.GetFeatureName()).Infof("couldn't cancel reservation %v", reservationID)
		}
	}
	e = centralSystem.CancelReservation(chargePointID, cb2, reservationID)
	if e != nil {
		logDefault(chargePointID, reservation.ReserveNowFeatureName).Errorf("couldn't send message: %v", e)
		return
	}