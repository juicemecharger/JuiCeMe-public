package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type jsonreq struct {
	Id     int      `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type jsonreply struct {
	Id      int         `json:"id"`
	Jsonrpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
}

func (handler *CentralSystemHandler) Listen(version string) {
	log.Printf("Starting API SERVER")
	handler.version = version
	m := mux.NewRouter()
	m.HandleFunc("/api", handler.api)
	m.HandleFunc("/", handler.error)
	log.Printf("Listening on Port 8080 on all interfaces")
	err := http.ListenAndServe("0.0.0.0:8080", m)
	if err != nil {
		log.Fatalf("Failed to start API SERVER %v", err)

	}
	if err == nil {
		log.Printf("stated and listening on 80")
	}
}

func (handler *CentralSystemHandler) error(w http.ResponseWriter, r *http.Request) {
	log.Printf("error 404 from %v", r.RemoteAddr)
	w.WriteHeader(http.StatusNotFound)
	fullstring := "OCPP-API-SERVER/" + handler.version
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Server", fullstring)
}

func (handler *CentralSystemHandler) api(w http.ResponseWriter, r *http.Request) {
	var reply jsonreply
	// START
	var req jsonreq
	_ = json.NewDecoder(r.Body).Decode(&req)
	reply.Id = req.Id
	reply.Jsonrpc = "2.0"
	switch req.Method {
	case "getChargePoints":
		chargepointlist := handler.GetChargePointList()
		reply.Result = chargepointlist
	case "getSystemState":
		statelist := handler.GetSystemState()
		reply.Result = statelist
	case "remoteStartTransaction":
		var idtag string
		chargePointID := req.Params[0]
		if len(req.Params) > 1 {
			idtag = req.Params[1]

		} else {
			idtag = "noIDSet"
		}
		reply.Result = "true"
		handler.SetChargePointRemoteStart(chargePointID, idtag)
	case "unlockConnector":
		var connectorID int
		confirmation := "false"
		timeout := 0
		if len(req.Params) > 1 {
			connectorID, _ = strconv.Atoi(req.Params[1])

		} else {
			connectorID = 1
		}
		handler.chargePoints[req.Params[0]].Connectors[connectorID].UnlockProgress = ""
		go handler.UnlockPort(req.Params[0], connectorID)
		for timeout < 10 {
			timeout++
			time.Sleep(5 * time.Millisecond)
			if handler.chargePoints[req.Params[0]].Connectors[connectorID].UnlockProgress != "" {
				confirmation = handler.chargePoints[req.Params[0]].Connectors[connectorID].UnlockProgress
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if confirmation != "" {
			reply.Result = confirmation
		} else {
			reply.Result = "ERROR"
		}
	default:
		reply.Result = "unknownMethod"
	}
	// END
	//
	//Setting headers
	fullstring := "OCPP-API-SERVER/" + handler.version
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Server", fullstring)

	//// writing
	err10 := json.NewEncoder(w).Encode(reply)
	if err10 != nil {
		log.Printf("error in reply")
		return
	}
	//log.Printf("closing api socket")
	//w.Write(output)
}
