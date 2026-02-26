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

// startWatcher runs a loop to periodically reload configuration
func startWatcher() {
	log.Printf("[Watcher] Monitoring directory: %s", appConfig.DefinitionsDir)
	for {
		refreshConfig()
		time.Sleep(30 * time.Second)
	}
}

// loadAndValidateAll reads all YAML files, applies inheritance, and validates integrity
func loadAndValidateAll() (*models.GlobalConfig, error) {
	finalCfg := &models.GlobalConfig{}

	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			if err := mergeFileToConfig(path, finalCfg); err != nil { return err }
		}
		return nil
	})
	if err != nil { return nil, err }

	processedCfg := processInheritance(finalCfg)
	if err := performCrossValidation(processedCfg); err != nil { return nil, err }

	return processedCfg, nil
}

// performCrossValidation checks for broken references between objects
func performCrossValidation(cfg *models.GlobalConfig) error {
	commands := make(map[string]bool)
	for _, c := range cfg.Commands { commands[c.ID] = true }

	contacts := make(map[string]bool)
	for _, c := range cfg.Contacts { contacts[c.ID] = true }

	hosts := make(map[string]bool)
	for _, h := range cfg.Hosts {
		hosts[h.ID] = true
		baseCmd := strings.Split(h.CheckCommand, "!")[0]
		if h.CheckCommand != "" && !commands[baseCmd] {
			return fmt.Errorf("host '%s' uses undefined command '%s'", h.ID, baseCmd)
		}
	}

	for _, s := range cfg.Services {
		if !hosts[s.HostName] {
			return fmt.Errorf("service '%s' bound to unknown host '%s'", s.ID, s.HostName)
		}
		baseCmd := strings.Split(s.CheckCommand, "!")[0]
		if s.CheckCommand != "" && !commands[baseCmd] {
			return fmt.Errorf("service '%s' uses undefined command '%s'", s.ID, baseCmd)
		}
	}
	return nil
}

// mergeFileToConfig unmarshals a single file into the global buffer
func mergeFileToConfig(path string, target *models.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	var temp models.GlobalConfig
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("YAML error in %s: %v", path, err)
	}
	target.Commands = append(target.Commands, temp.Commands...)
	target.Contacts = append(target.Contacts, temp.Contacts...)
	target.Hosts = append(target.Hosts, temp.Hosts...)
	target.Services = append(target.Services, temp.Services...)
	target.HostGroups = append(target.HostGroups, temp.HostGroups...)
	target.ServiceGroups = append(target.ServiceGroups, temp.ServiceGroups...)
	return nil
}

// processInheritance applies properties from templates to registered objects
func processInheritance(cfg *models.GlobalConfig) *models.GlobalConfig {
	// Host inheritance logic
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
					if len(h.Contacts) == 0 { h.Contacts = t.Contacts }
				}
			}
			actHosts = append(actHosts, h)
		}
	}
	// Service inheritance logic
	sTpl := make(map[string]models.Service)
	for _, s := range cfg.Services {
		if s.Register != nil && !*s.Register { sTpl[s.ID] = s }
	}
	var actSvc []models.Service
	for _, s := range cfg.Services {
		if s.Register == nil || *s.Register {
			if s.Use != "" {
				if t, ok := sTpl[s.Use]; ok {
					if s.NormalInterval == 0 { s.NormalInterval = t.NormalInterval }
					if s.RetryInterval == 0 { s.RetryInterval = t.RetryInterval }
					if s.MaxAttempts == 0 { s.MaxAttempts = t.MaxAttempts }
				}
			}
			actSvc = append(actSvc, s)
		}
	}
	cfg.Hosts = actHosts
	cfg.Services = actSvc
	return cfg
}

// refreshConfig triggers re-loading and sends it to the Scheduler
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Validation failed: %v", err)
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
		log.Printf("[Watcher] Successfully synced %d hosts and %d services", len(cfg.Hosts), len(cfg.Services))
		resp.Body.Close()
	} else {
		syncSuccess = false
		log.Printf("[Watcher] Sync failed to %s", url)
	}
}
