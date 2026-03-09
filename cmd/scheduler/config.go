package main

import (
	"encoding/json"
	"log"
	"os"

	"shinsakuto/pkg/logger"
)

// SchedConfig defines the runtime parameters for the Scheduler
type SchedConfig struct {
	APIAddress     string   `json:"api_address"`
	APIPort        int      `json:"api_port"`
	ReactionnerURL string   `json:"reactionner_url"`
	BrokerEnabled  bool     `json:"broker_enabled"`
	BrokerURLs     []string `json:"broker_urls"`
	StateFile      string   `json:"state_file"`
	LogFile       string   `json:"log_file"`
	HistoryLog     string   `json:"history_log"`
	Debug          bool     `json:"debug"`
}

// loadConfig reads and parses the JSON configuration file
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// initLoggers initializes technical and history logs
func initLoggers() {
	// Global technical logger
	logger.Setup(appConfig.LogFile, appConfig.Debug)

	// Status transition history logger
	if appConfig.HistoryLog != "" {
		f, err := os.OpenFile(appConfig.HistoryLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			statusLogger = log.New(f, "", log.LstdFlags)
		}
	}
}

// logStateChange writes state transitions to the history log
func logStateChange(entityType, id, stateStr, output string) {
	if statusLogger != nil {
		statusLogger.Printf("%-7s | %-20s | %-8s | %s", entityType, id, stateStr, output)
	}
}
