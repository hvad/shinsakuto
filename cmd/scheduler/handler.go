package main

import (
	"encoding/json"
	"net/http"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// syncAllHandler receives full config from Arbiter and rebuilds maps
func syncAllHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info("Received SyncAll request from Arbiter")
	var cfg models.GlobalConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		logger.Info("Sync error: Failed to decode JSON")
		http.Error(w, "Bad JSON", 400)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Rebuild Hosts while preserving state
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

	// Rebuild Services while preserving state
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
	logger.Info("SyncAll successful: %d hosts, %d services", len(hosts), len(services))
	w.WriteHeader(http.StatusOK)
}

// popTaskHandler serves the next task reaching its check interval
func popTaskHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	// Prioritize Host checks
	for _, h := range hosts {
		if h.CheckCommand != "" && now.After(h.NextCheck) {
			h.NextCheck = now.Add(2 * time.Minute) 
			json.NewEncoder(w).Encode(models.CheckTask{ID: "HOST:" + h.ID, Command: h.CheckCommand})
			return
		}
	}
	// Service checks
	for _, s := range services {
		if s.CheckCommand != "" && now.After(s.NextCheck) {
			s.NextCheck = now.Add(1 * time.Minute)
			json.NewEncoder(w).Encode(models.CheckTask{ID: s.ID, Command: s.CheckCommand})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushResultHandler queues results asynchronously to prevent lock contention
func pushResultHandler(w http.ResponseWriter, r *http.Request) {
	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return
	}

	select {
	case resultQueue <- res:
		w.WriteHeader(http.StatusAccepted) // 202 Accepted: queued for processing
	default:
		logger.Info("[WARNING] resultQueue full, dropping result for %s", res.ID)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// statusHandler returns the current in-memory state
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"hosts": hosts, "services": services})
}
