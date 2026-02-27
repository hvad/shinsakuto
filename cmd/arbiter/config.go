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

// ArbiterLocalConfig stores internal settings for this specific Arbiter instance
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
	configMutex   sync.RWMutex
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads the config.json file and initializes the logger
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	if err := json.Unmarshal(data, &appConfig); err != nil { return err }

	// If LogFile is provided, output logs to both file and standard output
	if appConfig.LogFile != "" {
		f, err := os.OpenFile(appConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil { return err }
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	}
	return nil
}
