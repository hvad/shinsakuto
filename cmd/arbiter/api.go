package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

var server *http.Server

// startAPI runs the HTTP status server
func startAPI() {
	port := appConfig.APIPort
	if port == 0 { port = 8083 }
	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, port)
	
	mux := http.NewServeMux()
	mux.HandleFunc("/status", statusHandler)

	server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("[API] Status server listening on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[API] Server failed: %v", err)
	}
}

// stopAPI shuts down the server with a 5-second timeout
func stopAPI() {
	log.Println("[API] Stopping status server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[API] Shutdown error: %v", err)
	}
}

// statusHandler reports inventory counts and sync state
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
