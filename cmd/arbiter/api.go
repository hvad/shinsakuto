package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"shinsakuto/pkg/models"
)

var server *http.Server

// startAPI configures the HTTP server with v1 routing
func startAPI() {
	port := appConfig.APIPort
	if port == 0 { port = 8083 }
	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, port)
	
	mux := http.NewServeMux()
	
	// REST API Version 1
	mux.HandleFunc("/v1/status", statusHandler)
	mux.HandleFunc("/v1/downtime", downtimeHandler)
	mux.HandleFunc("/v1/downtime/", downtimeDeleteHandler)

	server = &http.Server{Addr: addr, Handler: mux}
	log.Printf("[API] Version 1 listening on %s", addr)
	server.ListenAndServe()
}

// downtimeHandler handles GET (list) and POST (create) for maintenance windows
func downtimeHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(downtimes)
		return
	}

	if r.Method == http.MethodPost {
		var d models.Downtime
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		d.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())
		downtimes = append(downtimes, d)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(d)
		go refreshConfig() // Trigger sync immediately
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// downtimeDeleteHandler deletes a downtime by its ID
func downtimeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/downtime/")
	configMutex.Lock()
	defer configMutex.Unlock()

	for i, d := range downtimes {
		if d.ID == id {
			downtimes = append(downtimes[:i], downtimes[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			go refreshConfig()
			return
		}
	}
	http.Error(w, "Downtime not found", http.StatusNotFound)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	res := map[string]interface{}{
		"api_version": "v1",
		"sync_status": syncSuccess,
		"last_sync":   lastSyncTime.Format(time.RFC3339),
		"counts": map[string]int{
			"hosts": len(currentConfig.Hosts),
			"services": len(currentConfig.Services),
			"active_downtimes": len(downtimes),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func stopAPI() {
	log.Println("[API] Shutting down...")
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	server.Shutdown(ctx)
}
