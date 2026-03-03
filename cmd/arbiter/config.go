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

// ArbiterLocalConfig represents the local configuration parameters.
type ArbiterLocalConfig struct {
	SchedulerURLs           []string `json:"scheduler_urls"`             
	SchedulerCoolOffMinutes int      `json:"scheduler_cool_off_minutes"` 
	DefinitionsDir          string   `json:"definitions_dir"`
	APIAddress              string   `json:"api_address"`
	APIPort                 int      `json:"api_port"`
	LogFile                 string   `json:"log_file"`
	Debug                   bool     `json:"debug"`
	HotReload               bool     `json:"hot_reload"`
	HotReloadDebounce       int      `json:"hot_reload_debounce"`
	SyncInterval            int      `json:"sync_interval"`
	HAEnabled               bool     `json:"ha_enabled"`
	RaftNodeID              string   `json:"raft_node_id"`
	RaftBindAddr            string   `json:"raft_bind_addr"`
	RaftDataDir             string   `json:"raft_data_dir"`
	BootstrapCluster        bool     `json:"bootstrap_cluster"`
	ClusterNodes            []string `json:"cluster_nodes"`
}

var (
	appConfig     ArbiterLocalConfig
	currentConfig models.GlobalConfig
	downtimes     []models.Downtime
	configMutex   sync.RWMutex
	lastSyncTime  time.Time
	syncSuccess   bool
)

// loadArbiterLocalConfig reads and parses the JSON configuration file.
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { 
		return err 
	}
	
	if err := json.Unmarshal(data, &appConfig); err != nil { 
		return err 
	}

	// Set default cool-off to 5 minutes if not specified in JSON
	if appConfig.SchedulerCoolOffMinutes <= 0 {
		appConfig.SchedulerCoolOffMinutes = 5
	}

	// Setup logging output
	var logWriter io.Writer = os.Stdout
	if appConfig.LogFile != "" {
		f, err := os.OpenFile(appConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			logWriter = io.MultiWriter(os.Stdout, f)
		}
	}
	log.SetOutput(logWriter)
	return nil
}
