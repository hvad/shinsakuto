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

// PollerConfig defines parameters for multi-scheduler high availability
type PollerConfig struct {
	SchedulerURLs []string `json:"scheduler_urls"`
	PollerID      string   `json:"poller_id"`
	Interval      int      `json:"interval_ms"`
	MaxConcurrent int      `json:"max_concurrent"`
	Debug         bool     `json:"debug"`
	LogResults    bool     `json:"log_results"`
	LogFile       string   `json:"log_file"`
}

var (
	appConfig  PollerConfig
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// loadConfig reads the JSON configuration and initializes dual-output logging
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	if err := json.Unmarshal(data, &appConfig); err != nil {
		return err
	}

	// Default to standard output
	var logWriter io.Writer = os.Stdout

	// If a log file is defined, use a MultiWriter to write to both stdout and file
	if appConfig.LogFile != "" {
		f, err := os.OpenFile(appConfig.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			logWriter = io.MultiWriter(os.Stdout, f)
		} else {
			fmt.Printf("[ERROR] Poller: Failed to open log file: %v\n", err)
		}
	}

	log.SetOutput(logWriter)
	return nil
}

// logDebug prints detailed technical logs if debug is enabled
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
