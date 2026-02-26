package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// startAPI initializes the status server
func startAPI() {
	port := appConfig.APIPort
	if port == 0 { port = 8083 }
	http.HandleFunc("/status", statusHandler)
	addr := fmt.Sprintf(":%d", port)
	log.Printf("[API] Status server listening on %s", addr)
	http.ListenAndServe(addr, nil)
}

// statusHandler returns a JSON report of the current Arbiter state
func statusHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	res := map[string]interface{}{
		"project": "shinsakuto",
		"component": "arbiter",
		"sync_status": syncSuccess,
		"last_sync": lastSyncTime.Format("15:04:05"),
		"inventory": map[string]int{
			"hosts":    len(currentConfig.Hosts),
			"services": len(currentConfig.Services),
			"commands": len(currentConfig.Commands),
			"contacts": len(currentConfig.Contacts),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
