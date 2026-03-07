package main

import (
	"encoding/json"
	"fmt"
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
// It also initializes the logging system based on the debug flag.
func loadArbiterLocalConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &appConfig); err != nil {
		return err
	}

	// Set default cool-off to 5 minutes if not specified
	if appConfig.SchedulerCoolOffMinutes <= 0 {
		appConfig.SchedulerCoolOffMinutes = 5
	}

	// Logging Initialization
	if appConfig.LogFile != "" {
		f, err := os.OpenFile(appConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			// By default, global logger writes ONLY to the file
			log.SetOutput(f)
		} else {
			// Fallback to stdout if file creation fails
			log.SetOutput(os.Stdout)
			log.Printf("[ERROR] Could not open log file %s: %v", appConfig.LogFile, err)
		}
	} else {
		// If no log file is defined, write everything to stdout
		log.SetOutput(os.Stdout)
	}

	return nil
}

// logArbiter handles conditional logging.
// It always writes to the log file (via the global logger).
// It writes to the terminal (os.Stdout) ONLY if Debug is enabled.
func logArbiter(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)

	// 1. Always write to the configured log output (the file)
	log.Print(msg)

	// 2. Write to terminal ONLY if debug mode is active and we are logging to a file
	// (If LogFile is empty, log.Print already wrote to Stdout)
	if appConfig.Debug && appConfig.LogFile != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// logFatal is used for critical errors that must always appear in the terminal.
func logFatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print(msg) // Write to file
	if appConfig.LogFile != "" {
		fmt.Fprintf(os.Stderr, "%s [FATAL] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
	os.Exit(1)
}
