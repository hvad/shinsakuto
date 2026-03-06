package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// SchedConfig defines the runtime parameters for the Scheduler
type SchedConfig struct {
	APIAddress     string `json:"api_address"`
	APIPort        int    `json:"api_port"`
	ReactionnerURL string `json:"reactionner_url"`
	BrokerEnabled  bool   `json:"broker_enabled"`
	BrokerURL      string `json:"broker_url"`
	StateFile      string `json:"state_file"`
	SystemLog      string `json:"system_log"`
	HistoryLog     string `json:"history_log"`
	Debug          bool   `json:"debug"`
}

// loadConfig reads and parses the JSON configuration file
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

// logDebug prints detailed traces to the system log
func logDebug(format string, v ...interface{}) {
	if appConfig.Debug {
		msg := fmt.Sprintf("[DEBUG] "+format, v...)
		log.Println(msg)
	}
}

// initLoggers initializes the system and history log files
func initLoggers() {
	// Initialize system logging output
	if appConfig.SystemLog != "" {
		f, err := os.OpenFile(appConfig.SystemLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			log.SetOutput(f)
		} else {
			fmt.Printf("Warning: Could not open system log file: %v\n", err)
		}
	}

	// Initialize history logging for state changes
	if appConfig.HistoryLog != "" {
		f, err := os.OpenFile(appConfig.HistoryLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			statusLogger = log.New(f, "", log.LstdFlags)
		} else {
			fmt.Printf("Warning: Could not open history log file: %v\n", err)
		}
	}
}

// logStateChange writes state transitions to the history log
func logStateChange(entityType, id, stateStr, output string) {
	if statusLogger != nil {
		statusLogger.Printf("%-7s | %-20s | %-8s | %s", entityType, id, stateStr, output)
	}
}
