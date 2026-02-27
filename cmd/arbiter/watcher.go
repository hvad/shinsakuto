package main

import (
	"bytes"
	"context"
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

// startWatcher monitors files until context cancellation
func startWatcher(ctx context.Context) {
	log.Printf("[Watcher] Monitoring directory: %s", appConfig.DefinitionsDir)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		refreshConfig()
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			log.Println("[Watcher] Stopping watcher loop...")
			return
		}
	}
}

// loadAndValidateAll performs a full reload and cross-validation
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

// performCrossValidation checks for broken references
func performCrossValidation(cfg *models.GlobalConfig) error {
	commands := make(map[string]bool)
	for _, c := range cfg.Commands { commands[c.ID] = true }
	periods := make(map[string]bool)
	for _, p := range cfg.TimePeriods { periods[p.ID] = true }
	hosts := make(map[string]bool)

	for _, h := range cfg.Hosts {
		hosts[h.ID] = true
		if h.CheckCommand != "" && !commands[strings.Split(h.CheckCommand, "!")[0]] {
			return fmt.Errorf("host '%s' uses undefined command", h.ID)
		}
		if h.CheckPeriod != "" && !periods[h.CheckPeriod] {
			return fmt.Errorf("host '%s' uses undefined period", h.ID)
		}
	}
	for _, s := range cfg.Services {
		if !hosts[s.HostName] {
			return fmt.Errorf("service '%s' refers to unknown host", s.ID)
		}
	}
	return nil
}

func mergeFileToConfig(path string, target *models.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	var temp models.GlobalConfig
	if err := yaml.Unmarshal(data, &temp); err != nil { return err }
	target.Commands = append(target.Commands, temp.Commands...)
	target.Contacts = append(target.Contacts, temp.Contacts...)
	target.TimePeriods = append(target.TimePeriods, temp.TimePeriods...)
	target.HostGroups = append(target.HostGroups, temp.HostGroups...)
	target.ServiceGroups = append(target.ServiceGroups, temp.ServiceGroups...)
	target.Hosts = append(target.Hosts, temp.Hosts...)
	target.Services = append(target.Services, temp.Services...)
	return nil
}

func processInheritance(cfg *models.GlobalConfig) *models.GlobalConfig {
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

func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Error: %v", err)
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
		log.Printf("[Watcher] Successfully synced to Scheduler")
		resp.Body.Close()
	} else {
		syncSuccess = false
		log.Printf("[Watcher] Sync failed")
	}
}
