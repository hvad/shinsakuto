package main

import (
	"encoding/json"
	"net/http"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// Global map to store command definitions for host check resolution
var commandLibrary = make(map[string]string)

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

	// Update the command library to resolve Host CheckCommands later
	commandLibrary = make(map[string]string)
	for _, cmd := range cfg.Commands {
		commandLibrary[cmd.ID] = cmd.CommandLine
	}

	// Rebuild Hosts while preserving their current runtime state
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

	// Rebuild Services while preserving their current runtime state
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

// popTaskHandler serves the next pending task. It now includes the command line and address.
func popTaskHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	// 1. Prioritize Host health checks
	for _, h := range hosts {
		if h.CheckCommand != "" && now.After(h.NextCheck) {
			h.NextCheck = now.Add(2 * time.Minute)

			// Resolve the command template from our library
			cmdLine := commandLibrary[h.CheckCommand]

			json.NewEncoder(w).Encode(map[string]string{
				"id":           h.ID,
				"command_line": cmdLine,
				"address":      h.Address, // From Host.Address
			})
			return
		}
	}

	// 2. Service checks
	for _, s := range services {
		// Access command via CommandDefinition.CommandLine
		if s.CommandDefinition.CommandLine != "" && now.After(s.NextCheck) {
			s.NextCheck = now.Add(1 * time.Minute)

			// Resolve host address using HostName
			addr := ""
			if h, ok := hosts[s.HostName]; ok {
				addr = h.Address
			}

			json.NewEncoder(w).Encode(map[string]string{
				"id":           s.ID,
				"command_line": s.CommandDefinition.CommandLine,
				"address":      addr,
			})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushResultHandler queues incoming results asynchronously
func pushResultHandler(w http.ResponseWriter, r *http.Request) {
	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return
	}

	select {
	case resultQueue <- res:
		w.WriteHeader(http.StatusAccepted)
	default:
		logger.Info("[WARNING] resultQueue full, dropping result for %s", res.ID)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// statusHandler returns the current global state
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"hosts": hosts, "services": services})
}
