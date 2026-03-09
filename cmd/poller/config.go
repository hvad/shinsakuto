package main

import (
	"encoding/json"
	"os"

	"shinsakuto/pkg/logger"
)

// PollerConfig defines the execution settings for a poller node
type PollerConfig struct {
	PollerID      string   `json:"poller_id"`      // Unique identifier for this node
	SchedulerURLs []string `json:"scheduler_urls"` // List of upstream schedulers
	IntervalMS    int      `json:"interval_ms"`    // Polling frequency in milliseconds
	MaxConcurrent int      `json:"max_concurrent"` // Max number of parallel checks
	LogFile       string   `json:"log_file"`       // Path to the log file (optional)
	Debug         bool     `json:"debug"`          // If true, enables tracing via logger.Info
}

var (
	// Global application configuration instance
	appConfig PollerConfig
)

// loadConfig reads the JSON configuration file from the specified path
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLogger passes configuration settings to the centralized logging package
func initLogger() {
	// Debug traces will only be written to the log file/stdout if appConfig.Debug is true
	logger.Setup(appConfig.LogFile, appConfig.Debug)
}
