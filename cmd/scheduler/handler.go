package main

import (
	"encoding/json"
	"net/http"
	"shinsakuto/pkg/models"
	"strings"
)

// syncAllHandler receives the GlobalConfig from the Arbiter
func syncAllHandler(w http.ResponseWriter, r *http.Request) {
	logDebug("Received SyncAll request from Arbiter")
	var cfg models.GlobalConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		logDebug("Failed to decode sync JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	logDebug("Processing %d hosts and %d services", len(cfg.Hosts), len(cfg.Services))
	
	newHosts := make(map[string]*models.Host)
	for _, h := range cfg.Hosts {
		hostCopy := h
		if old, exists := hosts[h.ID]; exists {
			hostCopy.IsUp = old.IsUp // Preserve current dynamic state
			hostCopy.Status = old.Status
		} else {
			hostCopy.IsUp = true // Default new hosts to UP
			logDebug("New host detected: %s", h.ID)
		}
		newHosts[h.ID] = &hostCopy
	}
	hosts = newHosts

	newServices := make(map[string]*models.Service)
	for _, s := range cfg.Services {
		serviceCopy := s
		newServices[s.ID] = &serviceCopy
	}
	services = newServices
	
	stateChanged = true
	logDebug("SyncAll completed successfully")
	w.WriteHeader(http.StatusOK)
}

// pushResultHandler processes check results received from Pollers
func pushResultHandler(w http.ResponseWriter, r *http.Request) {
	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		logDebug("Failed to decode check result: %v", err)
		return
	}

	logDebug("Received result for %s (Status: %d)", res.ID, res.Status)

	mu.Lock()
	if strings.HasPrefix(res.ID, "HOST:") {
		handleHostResult(res)
	} else {
		handleServiceResult(res)
	}
	stateChanged = true
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// statusHandler returns the current in-memory monitoring state as JSON
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hosts":    hosts,
		"services": services,
	})
}
