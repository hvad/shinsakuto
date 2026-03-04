package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"shinsakuto/pkg/models"
)

// SchedConfig defines the internal configuration for the Scheduler
type SchedConfig struct {
	APIAddress     string `json:"api_address"`
	APIPort        int    `json:"api_port"`
	ReactionnerURL string `json:"reactionner_url"`
	StateFile      string `json:"state_file"`
	SystemLog      string `json:"system_log"`
	HistoryLog     string `json:"history_log"`
	Debug          bool   `json:"debug"`
}

var (
	appConfig     SchedConfig
	hosts         = make(map[string]*models.Host)
	services      = make(map[string]*models.Service)
	mu            sync.RWMutex
	statusLogger  *log.Logger
	stateChanged  bool
	startTime     time.Time
	httpClient    = &http.Client{Timeout: 5 * time.Second}
)

// loadConfig reads the JSON configuration file from disk
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	return json.Unmarshal(data, &appConfig)
}

// logDebug prints formatted messages to the terminal if debug mode is active
func logDebug(format string, v ...interface{}) {
	if appConfig.Debug {
		msg := fmt.Sprintf("[DEBUG] "+format, v...)
		fmt.Println(msg) // Direct terminal output for developer visibility
		log.Println(msg) // Also recorded in the system log
	}
}

// initLoggers sets up system logging and history tracking
func initLoggers() {
	if appConfig.SystemLog != "" {
		f, err := os.OpenFile(appConfig.SystemLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil { log.SetOutput(f) }
	}

	if appConfig.HistoryLog != "" {
		f, err := os.OpenFile(appConfig.HistoryLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil { log.Fatalf("Critical: failed to open history log: %v", err) }
		statusLogger = log.New(f, "", log.LstdFlags)
	}
}

// logStateChange records transition events in the history log
func logStateChange(entityType, id, stateStr, output string) {
	if statusLogger != nil {
		statusLogger.Printf("%-7s | %-20s | %-8s | %s", entityType, id, stateStr, output)
	}
}
