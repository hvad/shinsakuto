package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"sync"
	"time"

	"shinsakuto/pkg/models"
)

// ArbiterLocalConfig stores the Arbiter's operational settings
type ArbiterLocalConfig struct {
	SchedulerURL    string `json:"scheduler_url"`
	DefinitionsDir  string `json:"definitions_dir"`
	APIPort         int    `json:"api_port"`
}

var (
	appConfig     ArbiterLocalConfig
	currentConfig models.GlobalConfig
	configMutex   sync.RWMutex
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads the JSON file for operational settings
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	return json.Unmarshal(data, &appConfig)
}

// daemonize restarts the process in the background
func daemonize(configPath string) {
	cmd := exec.Command(os.Args[0], "-config", configPath)
	cmd.Env = append(os.Environ(), "ARBITER_DAEMON=true")
	if err := cmd.Start(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
