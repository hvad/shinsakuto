package main

import (
	"encoding/json"
	"os"

	"shinsakuto/pkg/logger"
)

// BrokerConfig defines the settings for data ingestion and storage
type BrokerConfig struct {
	Address         int    `json:"address"`
	Port            int    `json:"port"`
	TSDBUrl         string `json:"tsdb_url"`          // VictoriaMetrics or InfluxDB write endpoint
	TSDBToken       string `json:"tsdb_token"`        // Optional Authorization token
	BatchSize       int    `json:"batch_size"`        // Number of points per write operation
	FlushIntervalMS int    `json:"flush_interval_ms"` // Max time before forcing a database write
	WorkerCount     int    `json:"worker_count"`      // Number of concurrent database writers
	LogFile         string `json:"log_file"`          // Path to the broker log file
	Debug           bool   `json:"debug"`             // Enable verbose terminal output
}

var appConfig BrokerConfig

// loadConfig reads and parses the JSON configuration file
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLogger initializes the centralized logger with broker settings.
// It delegates the logic to the shared pkg/logger.
func initLogger() {
	// Configures the shared logger to write to the specified file
	// and toggle terminal output based on the debug flag.
	logger.Setup(appConfig.LogFile, appConfig.Debug)
}
