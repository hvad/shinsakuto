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

// startWatcher runs a loop that periodically reloads the configuration
func startWatcher() {
	log.Printf("[Watcher] Monitoring directory: %s", appConfig.DefinitionsDir)
	for {
		refreshConfig()
		time.Sleep(30 * time.Second)
	}
}

// loadAndValidateAll discovers YAML files, processes inheritance, and runs integrity checks
func loadAndValidateAll() (*models.GlobalConfig, error) {
	finalCfg := &models.GlobalConfig{}
	
	// Traverse the directory to find and merge all YAML files
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			if err := mergeFileToConfig(path, finalCfg); err != nil { return err }
		}
		return nil
	})
	if err != nil { return nil, err }

	// Resolve template logic (inheritance)
	processedCfg := processInheritance(finalCfg)
	
	// Perform cross-referential integrity checks
	if err := performCrossValidation(processedCfg); err != nil { return nil, err }
	
	return processedCfg, nil
}

// performCrossValidation ensures that IDs referenced (Commands, Periods, Hosts) actually exist
func performCrossValidation(cfg *models.GlobalConfig) error {
	commands := make(map[string]bool)
	for _, c := range cfg.Commands { commands[c.ID] = true }
	
	periods := make(map[string]bool)
	for _, p := range cfg.TimePeriods { periods[p.ID] = true }
	
	hosts := make(map[string]bool)
	for _, h := range cfg.Hosts {
		hosts[h.ID] = true
		// Validate Host Check Command
		if h.CheckCommand != "" {
			cmdID := strings.Split(h.CheckCommand, "!")[0]
			if !commands[cmdID] {
				return fmt.Errorf("host '%s' uses undefined command '%s'", h.ID, cmdID)
			}
		}
		// Validate Host TimePeriod
		if h.CheckPeriod != "" && !periods[h.CheckPeriod] {
			return fmt.Errorf("host '%s' uses undefined check_period '%s'", h.ID, h.CheckPeriod)
		}
	}

	for _, s := range cfg.Services {
		// Ensure service belongs to a valid host
		if !hosts[s.HostName] {
			return fmt.Errorf("service '%s' refers to unknown host '%s'", s.ID, s.HostName)
		}
		// Validate Service Check Command
		if s.CheckCommand != "" {
			cmdID := strings.Split(s.CheckCommand, "!")[0]
			if !commands[cmdID] {
				return fmt.Errorf("service '%s' uses undefined command '%s'", s.ID, cmdID)
			}
		}
	}
	return nil
}

// mergeFileToConfig unmarshals YAML content into the temporary GlobalConfig object
func mergeFileToConfig(path string, target *models.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	var temp models.GlobalConfig
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("YAML error in %s: %v", path, err)
	}
	target.Commands = append(target.Commands, temp.Commands...)
	target.Contacts = append(target.Contacts, temp.Contacts...)
	target.TimePeriods = append(target.TimePeriods, temp.TimePeriods...)
	target.HostGroups = append(target.HostGroups, temp.HostGroups...)
	target.ServiceGroups = append(target.ServiceGroups, temp.ServiceGroups...)
	target.Hosts = append(target.Hosts, temp.Hosts...)
	target.Services = append(target.Services, temp.Services...)
	return nil
}

// processInheritance copies attributes from templates (register=false) to active objects
func processInheritance(cfg *models.GlobalConfig) *models.GlobalConfig {
	// Host Template resolution
	hTpl := make(map[string]models.Host)
	for _, h := range cfg.Hosts {
		if h.Register != nil && !*h.Register { hTpl[h.ID] = h }
	}
	var actHosts []models.Host
	for _, h := range cfg.Hosts {
		if h.Register == nil || *h.Register {
			if h.Use != "" {
				if t, ok := hTpl[h.Use]; ok {
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.CheckPeriod == "" { h.CheckPeriod = t.CheckPeriod }
				}
			}
			actHosts = append(actHosts, h)
		}
	}
	cfg.Hosts = actHosts
	return cfg
}

// refreshConfig reloads the disk configuration and pushes it to the Scheduler
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Configuration error: %v", err)
		syncSuccess = false
		return
	}
	configMutex.Lock()
	currentConfig = *cfg
	configMutex.Unlock()

	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/sync-all"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err == nil && resp.StatusCode == 200 {
		lastSyncTime = time.Now()
		syncSuccess = true
		log.Printf("[Watcher] Successfully synced %d objects with Scheduler", 
			len(cfg.Hosts)+len(cfg.Services))
		resp.Body.Close()
	} else {
		syncSuccess = false
		log.Printf("[Watcher] Sync failed: Scheduler at %s is unreachable", url)
	}
}
