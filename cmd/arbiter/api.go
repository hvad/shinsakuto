package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// startAPI launches the HTTP server for status monitoring
func startAPI() {
	port := appConfig.APIPort
	if port == 0 { port = 8083 }

	http.HandleFunc("/status", statusHandler)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[API] Status server started on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[API] Fatal error on port %d: %v", port, err)
	}
}

// statusHandler returns current sync status and object counts
func statusHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()

	res := map[string]interface{}{
		"status":        "RUNNING",
		"sync_ok":       syncSuccess,
		"last_sync":     lastSyncTime.Format("2006-01-02 15:04:05"),
		"objects": map[string]int{
			"hosts":    len(currentConfig.Hosts),
			"services": len(currentConfig.Services),
			"commands": len(currentConfig.Commands),
			"contacts": len(currentConfig.Contacts),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
