package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
	"shinsakuto/pkg/models"
)

// ArbiterLocalConfig stores the internal settings for the binary
type ArbiterLocalConfig struct {
	SchedulerURL   string `json:"scheduler_url"`
	DefinitionsDir string `json:"definitions_dir"`
	APIPort        int    `json:"api_port"`
}

var (
	appConfig     ArbiterLocalConfig
	currentConfig models.GlobalConfig
	configMutex   sync.RWMutex
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads the config.json file
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	return json.Unmarshal(data, &appConfig)
}
