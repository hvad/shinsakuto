package main

import (
	"encoding/json"
	"os"
	"shinsakuto/pkg/models"
)

// saveState serializes current monitoring maps to the designated state file
func saveState() {
	mu.RLock()
	defer mu.RUnlock()

	logDebug("Persisting monitoring state to %s", appConfig.StateFile)
	data, err := json.MarshalIndent(map[string]interface{}{
		"hosts":    hosts,
		"services": services,
	}, "", "  ")

	if err != nil {
		logDebug("Error marshaling state: %v", err)
		return
	}

	if err := os.WriteFile(appConfig.StateFile, data, 0644); err != nil {
		logDebug("Error writing state file: %v", err)
	} else {
		logDebug("State saved successfully (%d bytes)", len(data))
	}
}

// loadState restores the monitoring engine's state from disk on startup
func loadState() {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(appConfig.StateFile)
	if err != nil { return }

	var st struct {
		Hosts    map[string]*models.Host    `json:"hosts"`
		Services map[string]*models.Service `json:"services"`
	}

	if err := json.Unmarshal(data, &st); err == nil {
		hosts = st.Hosts
		services = st.Services
		logDebug("State restored: %d hosts, %d services", len(hosts), len(services))
	}
}
