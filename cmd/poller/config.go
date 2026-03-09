package main

import (
	"encoding/json"
	"os"

	"shinsakuto/pkg/logger"
)

// PollerConfig defines the operational settings for the poller node
type PollerConfig struct {
	PollerID      string   `json:"poller_id"`      // Unique ID for this poller
	SchedulerURLs []string `json:"scheduler_urls"` // List of upstream Schedulers
	IntervalMS    int      `json:"interval_ms"`    // Delay between poll cycles
	MaxConcurrent int      `json:"max_concurrent"` // Max parallel check routines
	LogFile       string   `json:"log_file"`       // Path to the log file
	Debug         bool     `json:"debug"`          // Toggle for verbose tracing
}

var (
	// Global variable to store loaded configuration
	appConfig PollerConfig
)

// loadConfig opens the specified JSON file and unmarshals it into appConfig
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLogger configures the logging package with the settings from config
func initLogger() {
	// Passing log file path and debug flag to the internal logger setup
	logger.Setup(appConfig.LogFile, appConfig.Debug)
}
