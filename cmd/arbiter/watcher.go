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

// Global HTTP client with timeout to prevent the watcher from hanging
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// startWatcher runs the infinite loop for configuration synchronization
func startWatcher(ctx context.Context) {
	// Immediate first run
	refreshConfig()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			refreshConfig()
		case <-ctx.Done():
			log.Println("[Watcher] Stopping configuration watcher...")
			return
		}
	}
}

// isRegistered helper to handle the default 'true' value for Register field
func isRegistered(reg *bool) bool {
	if reg == nil {
		return true // Default behavior: if not specified, it's an active object
	}
	return *reg
}

// loadAndValidateAll crawls the directory, parses YAMLs and handles inheritance
func loadAndValidateAll() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}

	// Walk through the definitions directory to find YAML files
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("error reading file %s: %v", path, err)
			}

			var temp models.GlobalConfig
			if err := yaml.Unmarshal(data, &temp); err != nil {
				return fmt.Errorf("YAML error in %s: %v", path, err)
			}

			// Aggregate all definitions
			raw.Commands = append(raw.Commands, temp.Commands...)
			raw.Contacts = append(raw.Contacts, temp.Contacts...)
			raw.TimePeriods = append(raw.TimePeriods, temp.TimePeriods...)
			raw.Hosts = append(raw.Hosts, temp.Hosts...)
			raw.Services = append(raw.Services, temp.Services...)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 1. Filter active objects and handle template inheritance
	final := processRegistryAndInheritance(raw)

	// 2. Perform cross-reference validation (e.g., Service -> Host existence)
	if err := performFullValidation(final); err != nil {
		return nil, err
	}

	return final, nil
}

// processRegistryAndInheritance separates templates (register: false) from active objects
func processRegistryAndInheritance(raw *models.GlobalConfig) *models.GlobalConfig {
	hTpl := make(map[string]models.Host)
	sTpl := make(map[string]models.Service)
	final := &models.GlobalConfig{Commands: raw.Commands}

	// Filter Contacts and TimePeriods (they don't support inheritance here, just registration)
	for _, c := range raw.Contacts {
		if isRegistered(c.Register) {
			final.Contacts = append(final.Contacts, c)
		}
	}
	for _, p := range raw.TimePeriods {
		if isRegistered(p.Register) {
			final.TimePeriods = append(final.TimePeriods, p)
		}
	}

	// Step A: Collect all Templates (register: false)
	for _, h := range raw.Hosts {
		if !isRegistered(h.Register) {
			hTpl[h.ID] = h
		}
	}
	for _, s := range raw.Services {
		if !isRegistered(s.Register) {
			sTpl[s.ID] = s
		}
	}

	// Step B: Process Active Hosts and apply 'use' inheritance
	for _, h := range raw.Hosts {
		if isRegistered(h.Register) {
			if h.Use != "" {
				if t, ok := hTpl[h.Use]; ok {
					// Inherit fields if they are empty in the child
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.CheckPeriod == "" { h.CheckPeriod = t.CheckPeriod }
					if len(h.Contacts) == 0 { h.Contacts = t.Contacts }
					if h.Address == "" { h.Address = t.Address }
				}
			}
			final.Hosts = append(final.Hosts, h)
		}
	}

	// Step C: Process Active Services and apply inheritance
	for _, s := range raw.Services {
		if isRegistered(s.Register) {
			if s.Use != "" {
				if t, ok := sTpl[s.Use]; ok {
					if s.CheckCommand == "" { s.CheckCommand = t.CheckCommand }
					if s.CheckPeriod == "" { s.CheckPeriod = t.CheckPeriod }
					if len(s.Contacts) == 0 { s.Contacts = t.Contacts }
				}
			}
			final.Services = append(final.Services, s)
		}
	}

	return final
}

// performFullValidation ensures the configuration logic is sound
func performFullValidation(cfg *models.GlobalConfig) error {
	hostMap := make(map[string]bool)
	for _, h := range cfg.Hosts {
		hostMap[h.ID] = true
	}

	for _, s := range cfg.Services {
		if !hostMap[s.HostName] {
			return fmt.Errorf("validation error: service '%s' references unknown or non-registered host '%s'", s.ID, s.HostName)
		}
	}
	return nil
}

// refreshConfig triggers the full load, maintenance calculation, and push to Scheduler
func refreshConfig() {
	log.Println("[Watcher] Refreshing configuration...")

	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Configuration error: %v", err)
		syncSuccess = false
		return
	}

	// Apply current maintenance status (Downtimes)
	configMutex.Lock()
	now := time.Now()
	
	// Track which hosts are in downtime to propagate to their services
	hostsInDt := make(map[string]bool)

	for i := range cfg.Hosts {
		for _, d := range downtimes {
			// Check if host matches and we are within the time window
			if d.HostName == cfg.Hosts[i].ID && d.ServiceID == "" {
				if now.After(d.StartTime) && now.Before(d.EndTime) {
					cfg.Hosts[i].InDowntime = true
					hostsInDt[cfg.Hosts[i].ID] = true
				}
			}
		}
	}

	// Apply downtime to services (either directly or inherited from host)
	for i := range cfg.Services {
		// Inherit from host
		if hostsInDt[cfg.Services[i].HostName] {
			cfg.Services[i].InDowntime = true
		}
		// Direct service downtime
		for _, d := range downtimes {
			if d.HostName == cfg.Services[i].HostName && d.ServiceID == cfg.Services[i].ID {
				if now.After(d.StartTime) && now.Before(d.EndTime) {
					cfg.Services[i].InDowntime = true
				}
			}
		}
	}

	cfg.Downtimes = downtimes
	currentConfig = *cfg
	configMutex.Unlock()

	// Push configuration to the Scheduler
	pushToScheduler(cfg)
}

// pushToScheduler sends the final GlobalConfig to the Scheduler API
func pushToScheduler(cfg *models.GlobalConfig) {
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("[Watcher] JSON Marshal error: %v", err)
		return
	}

	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[Watcher] Failed to reach Scheduler at %s: %v", url, err)
		syncSuccess = false
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("[Watcher] Successfully synced %d hosts and %d services to Scheduler", len(cfg.Hosts), len(cfg.Services))
		syncSuccess = true
		lastSyncTime = time.Now()
	} else {
		log.Printf("[Watcher] Scheduler returned error status: %d", resp.StatusCode)
		syncSuccess = false
	}
}
