package main

import (
	"encoding/json"
	"log"
	"os"
)

// Config holds the Reactionner runtime parameters
type Config struct {
	APIPort   int        `json:"api_port"`
	Debug     bool       `json:"debug"`
	SystemLog string     `json:"system_log"`
	AlertsLog string     `json:"alerts_log"`
	SMTP      SMTPConfig `json:"smtp"`
	
	// HA / Raft Configuration
	HAEnabled        bool     `json:"ha_enabled"`
	RaftNodeID       string   `json:"raft_node_id"`
	RaftBindAddr     string   `json:"raft_bind_addr"`
	RaftDataDir      string   `json:"raft_data_dir"`
	BootstrapCluster bool     `json:"bootstrap_cluster"`
	ClusterNodes     []string `json:"cluster_nodes"`
}

type SMTPConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

var (
	appConfig       Config
	systemLogger    *log.Logger
	alertLogger     *log.Logger
	maintenances    = make(map[string]int64) 
	acknowledgments = make(map[string]bool)
)

// loadConfig reads the JSON configuration file
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	return json.Unmarshal(data, &appConfig)
}

// initLoggers initializes system and alert log files
func initLoggers() {
	// Technical system logger
	if appConfig.SystemLog != "" {
		f, _ := os.OpenFile(appConfig.SystemLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		systemLogger = log.New(f, "SYS: ", log.LstdFlags)
	} else {
		systemLogger = log.New(os.Stdout, "SYS: ", log.LstdFlags)
	}

	// Dedicated alert history logger
	if appConfig.AlertsLog != "" {
		f, _ := os.OpenFile(appConfig.AlertsLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		alertLogger = log.New(f, "ALERT: ", log.LstdFlags)
	} else {
		alertLogger = log.New(os.Stdout, "ALERT: ", log.LstdFlags)
	}
}

// logDebug provides verbose technical traces if Debug is enabled
func logDebug(msg string, args ...interface{}) {
	if appConfig.Debug {
		systemLogger.Printf("[DEBUG] "+msg, args...)
	}
}
