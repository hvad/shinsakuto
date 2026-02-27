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

func startAPI() {
	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", statusHandler)
	mux.HandleFunc("/v1/downtime", downtimeHandler)
	mux.HandleFunc("/v1/downtime/", downtimeDeleteHandler)

	server = &http.Server{Addr: addr, Handler: mux}
	log.Printf("[API] Server listening on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[API] Server error: %v", err)
	}
}

func stopAPI() {
	log.Println("[API] Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"synced":         syncSuccess,
		"last_sync":      lastSyncTime.Format(time.RFC3339),
		"active_hosts":   len(currentConfig.Hosts),
		"active_services": len(currentConfig.Services),
	})
}

func downtimeHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if r.Method == http.MethodPost {
		var d models.Downtime
		json.NewDecoder(r.Body).Decode(&d)
		d.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())
		downtimes = append(downtimes, d)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(d)
		go refreshConfig()
		return
	}
	json.NewEncoder(w).Encode(downtimes)
}

func downtimeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete { return }
	id := strings.TrimPrefix(r.URL.Path, "/v1/downtime/")
	configMutex.Lock()
	defer configMutex.Unlock()

	for i, d := range downtimes {
		if d.ID == id {
			downtimes = append(downtimes[:i], downtimes[i+1:]...)
			w.WriteHeader(204)
			go refreshConfig()
			return
		}
	}
	http.Error(w, "Not found", 404)
}
