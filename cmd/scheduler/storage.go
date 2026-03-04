package main

import (
	"encoding/json"
	"os"
	"shinsakuto/pkg/models" // Using the provided models
)

// saveState serializes the current host and service status to disk
func saveState() {
	mu.RLock()
	defer mu.RUnlock()

	logDebug("Persisting state to disk...")
	data, err := json.MarshalIndent(map[string]interface{}{
		"hosts":    hosts,
		"services": services,
	}, "", "  ")

	if err == nil {
		os.WriteFile(appConfig.StateFile, data, 0644)
	}
}

// loadState restores the state from the JSON file on startup
func loadState() {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(appConfig.StateFile)
	if err != nil {
		return
	}

	var st struct {
		Hosts    map[string]*models.Host    `json:"hosts"`
		Services map[string]*models.Service `json:"services"`
	}

	if err := json.Unmarshal(data, &st); err == nil {
		hosts, services = st.Hosts, st.Services
		logDebug("State file loaded: %d hosts, %d services restored", len(hosts), len(services))
	}
}
