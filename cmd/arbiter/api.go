package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// startAPI starts the HTTP server on the configured address and port
func startAPI() {
	port := appConfig.APIPort
	if port == 0 { port = 8083 }
	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, port)
	
	http.HandleFunc("/status", statusHandler)
	log.Printf("[API] Status server listening on %s", addr)
	
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[API] Fatal error: %v", err)
	}
}

// statusHandler returns a full JSON report of currently loaded objects and sync status
func statusHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	
	res := map[string]interface{}{
		"project":     "shinsakuto",
		"component":   "arbiter",
		"sync_status": syncSuccess,
		"last_sync":   lastSyncTime.Format("2006-01-02 15:04:05"),
		"inventory": map[string]int{
			"hosts":          len(currentConfig.Hosts),
			"services":       len(currentConfig.Services),
			"commands":       len(currentConfig.Commands),
			"contacts":       len(currentConfig.Contacts),
			"time_periods":   len(currentConfig.TimePeriods),
			"host_groups":    len(currentConfig.HostGroups),
			"service_groups": len(currentConfig.ServiceGroups),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
