package main

import (
	"encoding/json"
	"net/http"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// syncAllHandler receives the full configuration from the Arbiter and rebuilds internal maps.
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

	// Rebuild Hosts while preserving their current runtime state.
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

	// Rebuild Services while preserving their current runtime state.
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
	logger.Always("SyncAll successful: %d hosts, %d services synced", len(hosts), len(services))
	w.WriteHeader(http.StatusOK)
}

// popTaskHandler serves the next pending task reaching its check interval.
// It sends the full Host or Service object so the Poller has access to the linked Command.
func popTaskHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	// 1. Prioritize Host health checks.
	for _, h := range hosts {
		if h.CheckCommand != "" && now.After(h.NextCheck) {
			h.NextCheck = now.Add(2 * time.Minute)
			// Return the full Host object; Poller will use h.ID and h.Address.
			json.NewEncoder(w).Encode(h)
			return
		}
	}
	// 2. Service checks.
	for _, s := range services {
		if s.CheckCommand != "" && now.After(s.NextCheck) {
			s.NextCheck = now.Add(1 * time.Minute)
			// Return the full Service object; Poller will use s.CommandDefinition.
			json.NewEncoder(w).Encode(s)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushResultHandler queues incoming results asynchronously to prevent lock contention.
func pushResultHandler(w http.ResponseWriter, r *http.Request) {
	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return
	}

	select {
	case resultQueue <- res:
		w.WriteHeader(http.StatusAccepted) // 202 Accepted: queued for processing.
	default:
		logger.Info("[WARNING] resultQueue full, dropping result for %s", res.ID)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// statusHandler returns the current global in-memory state as JSON.
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"hosts": hosts, "services": services})
}
