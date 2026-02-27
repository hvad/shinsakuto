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
	SchedulerURL       string `json:"scheduler_url"`
	DefinitionsDir     string `json:"definitions_dir"`
	APIAddress         string `json:"api_address"`
	APIPort            int    `json:"api_port"`
	LogFile            string `json:"log_file"`
	HotReload          bool   `json:"hot_reload"`
	HotReloadDebounce  int    `json:"hot_reload_debounce"` 
	SyncInterval       int    `json:"sync_interval"`        
}

var (
	appConfig     ArbiterLocalConfig
	currentConfig models.GlobalConfig
	downtimes     []models.Downtime
	configMutex   sync.RWMutex
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads the JSON config file and initializes logging
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	if err := json.Unmarshal(data, &appConfig); err != nil { return err }

	if appConfig.SyncInterval == 0 { appConfig.SyncInterval = 60 }
	if appConfig.HotReloadDebounce == 0 { appConfig.HotReloadDebounce = 2 }

	if appConfig.LogFile != "" {
		f, _ := os.OpenFile(appConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}
	return nil
}
