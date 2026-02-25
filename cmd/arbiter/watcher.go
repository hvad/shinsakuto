package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shinsakuto/pkg/models"
	"gopkg.in/yaml.v3"
)

// startWatcher runs the infinite loop to check for config updates
func startWatcher() {
	log.Printf("[Watcher] Monitoring directory: %s", appConfig.DefinitionsDir)
	for {
		refreshConfig()
		time.Sleep(30 * time.Second)
	}
}

// loadAndValidateAll scans directories and parses YAML files
func loadAndValidateAll() (*models.GlobalConfig, error) {
	finalCfg := &models.GlobalConfig{}

	// Recursive walk to support split folders (commands/, hosts/, etc.)
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		// Only process .yaml or .yml files
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			if err := mergeFileToConfig(path, finalCfg); err != nil {
				return err
			}
		}
		return nil
	})
	
	if err != nil { return nil, err }
	return processInheritance(finalCfg), nil
}

// mergeFileToConfig unmarshals YAML data into the global structure
func mergeFileToConfig(path string, target *models.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	
	var temp models.GlobalConfig
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("invalid YAML in %s: %v", path, err)
	}
	
	target.Commands = append(target.Commands, temp.Commands...)
	target.Contacts = append(target.Contacts, temp.Contacts...)
	target.Hosts = append(target.Hosts, temp.Hosts...)
	target.Services = append(target.Services, temp.Services...)
	return nil
}

// processInheritance applies template properties using the 'use' keyword
func processInheritance(cfg *models.GlobalConfig) *models.GlobalConfig {
	hostTemplates := make(map[string]models.Host)
	for _, h := range cfg.Hosts {
		if h.Register != nil && !*h.Register {
			hostTemplates[h.ID] = h
		}
	}

	var activeHosts []models.Host
	for _, h := range cfg.Hosts {
		// Keep objects only if register is true or omitted
		if h.Register == nil || *h.Register {
			if h.Use != "" {
				if tpl, ok := hostTemplates[h.Use]; ok {
					if h.CheckCommand == "" { h.CheckCommand = tpl.CheckCommand }
					if h.Address == "" { h.Address = tpl.Address }
					if len(h.Contacts) == 0 { h.Contacts = tpl.Contacts }
				}
			}
			activeHosts = append(activeHosts, h)
		}
	}
	cfg.Hosts = activeHosts
	return cfg
}

// refreshConfig triggers the loading process and sends it to the Scheduler
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Config error, sync aborted: %v", err)
		syncSuccess = false
		return
	}

	configMutex.Lock()
	currentConfig = *cfg
	configMutex.Unlock()

	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/sync-all"
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(data))
	
	if err == nil && resp.StatusCode == 200 {
		lastSyncTime = time.Now()
		syncSuccess = true
		resp.Body.Close()
	} else {
		syncSuccess = false
		log.Printf("[Watcher] Failed to send config to Scheduler: %v", err)
	}
}
