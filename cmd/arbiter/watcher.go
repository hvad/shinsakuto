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

// startWatcher initializes the monitoring loop with a ticker
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
			log.Println("[Watcher] Context cancelled, stopping...")
			return
		}
	}
}

// loadAndValidateAll parses YAML files and runs integrity checks
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

	// Resolve templates and inheritance
	processedCfg := processInheritance(finalCfg)
	
	// Perform cross-object validation (commands, periods, contacts...)
	if err := performDeepValidation(processedCfg); err != nil { return nil, err }
	
	return processedCfg, nil
}

// performDeepValidation ensures all references between objects are valid
func performDeepValidation(cfg *models.GlobalConfig) error {
	// Map existing objects for O(1) lookup during cross-validation
	commands := make(map[string]bool)
	for _, c := range cfg.Commands { 
		if c.ID == "" { return fmt.Errorf("found command with empty ID") }
		commands[c.ID] = true 
	}

	periods := make(map[string]bool)
	for _, p := range cfg.TimePeriods { 
		if p.ID == "" { return fmt.Errorf("found timeperiod with empty ID") }
		periods[p.ID] = true 
	}

	contacts := make(map[string]bool)
	for _, c := range cfg.Contacts { 
		if c.ID == "" { return fmt.Errorf("found contact with empty ID") }
		contacts[c.ID] = true 
	}

	hosts := make(map[string]bool)
	for _, h := range cfg.Hosts {
		hosts[h.ID] = true
		// Validate Host Check Command
		if h.CheckCommand != "" {
			cmdName := strings.Split(h.CheckCommand, "!")[0]
			if !commands[cmdName] { return fmt.Errorf("host '%s' uses undefined command '%s'", h.ID, cmdName) }
		}
		// Validate Host Time Periods
		if h.CheckPeriod != "" && !periods[h.CheckPeriod] { return fmt.Errorf("host '%s' uses undefined check_period '%s'", h.ID, h.CheckPeriod) }
		if h.NotificationPeriod != "" && !periods[h.NotificationPeriod] { return fmt.Errorf("host '%s' uses undefined notification_period '%s'", h.ID, h.NotificationPeriod) }
		
		// Validate Host Contacts
		for _, cName := range h.Contacts {
			if !contacts[cName] { return fmt.Errorf("host '%s' refers to undefined contact '%s'", h.ID, cName) }
		}
	}

	for _, s := range cfg.Services {
		if !hosts[s.HostName] { return fmt.Errorf("service '%s' refers to unknown host '%s'", s.ID, s.HostName) }
		
		// Validate Service Check Command
		if s.CheckCommand != "" {
			cmdName := strings.Split(s.CheckCommand, "!")[0]
			if !commands[cmdName] { return fmt.Errorf("service '%s' uses undefined command '%s'", s.ID, cmdName) }
		}
		// Validate Service Time Periods
		if s.CheckPeriod != "" && !periods[s.CheckPeriod] { return fmt.Errorf("service '%s' uses undefined check_period '%s'", s.ID, s.CheckPeriod) }
		
		// Validate Service Contacts
		for _, cName := range s.Contacts {
			if !contacts[cName] { return fmt.Errorf("service '%s' refers to undefined contact '%s'", s.ID, cName) }
		}
	}
	return nil
}

// refreshConfig triggers a reload and sends data to the Scheduler
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Validation failed: %v", err)
		syncSuccess = false
		return
	}

	// Calculate Downtime propagation logic
	configMutex.Lock()
	now := time.Now()
	hostsInDt := make(map[string]bool)
	for i := range cfg.Hosts {
		for _, d := range downtimes {
			if d.HostName == cfg.Hosts[i].ID && d.ServiceID == "" && now.After(d.StartTime) && now.Before(d.EndTime) {
				cfg.Hosts[i].InDowntime = true
				hostsInDt[cfg.Hosts[i].ID] = true
			}
		}
	}
	for i := range cfg.Services {
		if hostsInDt[cfg.Services[i].HostName] { cfg.Services[i].InDowntime = true; continue }
		for _, d := range downtimes {
			if d.HostName == cfg.Services[i].HostName && d.ServiceID == cfg.Services[i].ID && now.After(d.StartTime) && now.Before(d.EndTime) {
				cfg.Services[i].InDowntime = true
			}
		}
	}
	cfg.Downtimes = downtimes
	currentConfig = *cfg
	configMutex.Unlock()

	// PUSH DATA TO SCHEDULER (The actual sync operation)
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/sync-all"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	
	if err == nil && resp.StatusCode == 200 {
		lastSyncTime = time.Now()
		syncSuccess = true
		log.Printf("[Watcher] Successfully pushed config to Scheduler at %s", url)
		resp.Body.Close()
	} else {
		syncSuccess = false
		log.Printf("[Watcher] Sync failed: %v", err)
	}
}

// Helper to merge files and handle templates (already provided in your snippets)
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
