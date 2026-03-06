package main

import (
	"encoding/json"
	"net/http"
	"shinsakuto/pkg/models"
	"strings"
	"time"
)

// syncAllHandler receives the full configuration from the Arbiter
func syncAllHandler(w http.ResponseWriter, r *http.Request) {
	logDebug("Received SyncAll request from Arbiter")
	var cfg models.GlobalConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		logDebug("Sync error: Failed to decode JSON")
		http.Error(w, "Bad JSON", 400)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Update Hosts while preserving dynamic state
	newHosts := make(map[string]*models.Host)
	for _, h := range cfg.Hosts {
		hCopy := h
		if old, exists := hosts[h.ID]; exists {
			hCopy.IsUp, hCopy.Status, hCopy.NextCheck = old.IsUp, old.Status, old.NextCheck
		} else {
			hCopy.IsUp, hCopy.NextCheck = true, time.Now()
		}
		newHosts[h.ID] = &hCopy
	}
	hosts = newHosts

	// Update Services while preserving dynamic state
	newServices := make(map[string]*models.Service)
	for _, s := range cfg.Services {
		sCopy := s
		if old, exists := services[s.ID]; exists {
			sCopy.NextCheck, sCopy.CurrentState = old.NextCheck, old.CurrentState
		} else {
			sCopy.NextCheck = time.Now()
		}
		newServices[s.ID] = &sCopy
	}
	services = newServices

	stateChanged = true
	logDebug("SyncAll successful: %d hosts, %d services", len(hosts), len(services))
	w.WriteHeader(http.StatusOK)
}

// popTaskHandler serves the next available task to a Poller
func popTaskHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	// Check Hosts first
	for _, h := range hosts {
		if h.CheckCommand != "" && now.After(h.NextCheck) {
			h.NextCheck = now.Add(2 * time.Minute) // Lock task to prevent duplicates
			json.NewEncoder(w).Encode(models.CheckTask{ID: "HOST:" + h.ID, Command: h.CheckCommand})
			return
		}
	}
	// Check Services
	for _, s := range services {
		if s.CheckCommand != "" && now.After(s.NextCheck) {
			s.NextCheck = now.Add(1 * time.Minute)
			json.NewEncoder(w).Encode(models.CheckTask{ID: s.ID, Command: s.CheckCommand})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushResultHandler processes results sent by Pollers
func pushResultHandler(w http.ResponseWriter, r *http.Request) {
	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return
	}
	logDebug("Result received for %s: status %d", res.ID, res.Status)

	mu.Lock()
	if strings.HasPrefix(res.ID, "HOST:") {
		handleHostResult(res)
	} else {
		handleServiceResult(res)
	}
	stateChanged = true
	mu.Unlock()
}

// statusHandler returns the current real-time state
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"hosts": hosts, "services": services})
}
