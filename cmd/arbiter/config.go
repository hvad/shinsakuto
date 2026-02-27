package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"
	"shinsakuto/pkg/models"
)

// ArbiterLocalConfig stores internal process settings
type ArbiterLocalConfig struct {
	SchedulerURL   string `json:"scheduler_url"`
	DefinitionsDir string `json:"definitions_dir"`
	APIAddress     string `json:"api_address"`
	APIPort        int    `json:"api_port"`
	LogFile        string `json:"log_file"`
}

var (
	appConfig     ArbiterLocalConfig
	currentConfig models.GlobalConfig
	downtimes     []models.Downtime // Memory-based storage for downtimes
	configMutex   sync.RWMutex      // Protects currentConfig and downtimes
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads the JSON config and initializes dual logging
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	if err := json.Unmarshal(data, &appConfig); err != nil { return err }

	if appConfig.LogFile != "" {
		f, err := os.OpenFile(appConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil { return err }
		// Logs both to stdout and the specified log file
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}
	return nil
}
