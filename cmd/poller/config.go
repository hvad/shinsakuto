package main

import (
	"encoding/json"
	"os"

	"shinsakuto/pkg/logger"
)

// PollerConfig defines the execution settings for a poller node
type PollerConfig struct {
	PollerID      string   `json:"poller_id"`
	SchedulerURLs []string `json:"scheduler_urls"`
	IntervalMS    int      `json:"interval_ms"`
	MaxConcurrent int      `json:"max_concurrent"`
	LogFile       string   `json:"log_file"`
	Debug         bool     `json:"debug"`
}

var (
	appConfig PollerConfig
)

// loadConfig reads the JSON configuration from disk
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLogger connects the poller to the centralized logging system
func initLogger() {
	logger.Setup(appConfig.LogFile, appConfig.Debug)
}
