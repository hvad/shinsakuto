package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

// BrokerConfig defines the settings for data ingestion and storage
type BrokerConfig struct {
	APIPort         int    `json:"api_port"`
	TSDBUrl         string `json:"tsdb_url"`           // VictoriaMetrics or InfluxDB write endpoint
	TSDBToken       string `json:"tsdb_token"`         // Optional Authorization token
	BatchSize       int    `json:"batch_size"`         // Number of points per write operation
	FlushIntervalMS int    `json:"flush_interval_ms"`  // Max time before forcing a database write
	WorkerCount     int    `json:"worker_count"`       // Number of concurrent database writers
	SystemLog       string `json:"system_log"`         // Path to the system log file
	Debug           bool   `json:"debug"`              // Enable verbose logging
}

var appConfig BrokerConfig

// loadConfig reads the JSON configuration file
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLogger sets up logging to both stdout and a file if specified
func initLogger() {
	var logWriter io.Writer = os.Stdout
	if appConfig.SystemLog != "" {
		f, err := os.OpenFile(appConfig.SystemLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			logWriter = io.MultiWriter(os.Stdout, f)
		}
	}
	log.SetOutput(logWriter)
}

// logDebug prints detailed traces if debug mode is active
func logDebug(format string, v ...interface{}) {
	if appConfig.Debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}
