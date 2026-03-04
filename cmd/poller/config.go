package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// PollerConfig defines connection, execution, and logging parameters
type PollerConfig struct {
	SchedulerURL  string `json:"scheduler_url"`
	PollerID      string `json:"poller_id"`
	Interval      int    `json:"interval_ms"`
	MaxConcurrent int    `json:"max_concurrent"`
	Debug         bool   `json:"debug"`
	LogResults    bool   `json:"log_results"`
	LogFile       string `json:"log_file"`
}

var (
	appConfig  PollerConfig
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// loadConfig reads the JSON configuration and initializes the multi-writer logger
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	if err := json.Unmarshal(data, &appConfig); err != nil {
		return err
	}

	// Initialize logging to both Terminal and File
	var logWriter io.Writer = os.Stdout
	if appConfig.LogFile != "" {
		// Open the log file in append mode, creating it if it doesn't exist
		f, err := os.OpenFile(appConfig.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			// MultiWriter sends data to both stdout and the log file
			logWriter = io.MultiWriter(os.Stdout, f)
		} else {
			fmt.Printf("[ERROR] Could not open log file: %v\n", err)
		}
	}
	log.SetOutput(logWriter)
	return nil
}

// logDebug prints detailed messages if debug mode is active
func logDebug(format string, v ...interface{}) {
	if appConfig.Debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// logResult prints check outcomes based on configuration flags
func logResult(format string, v ...interface{}) {
	if appConfig.LogResults || appConfig.Debug {
		log.Printf("[RESULT] "+format, v...)
	}
}
