package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shinsakuto/pkg/models"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Global HTTP client with timeout to prevent hanging connections
var httpClient = &http.Client{Timeout: 10 * time.Second}

// startWatcher initializes the periodic synchronization and the Hot Reload watcher
func startWatcher(ctx context.Context) {
	// First load on startup
	refreshConfig()

	// 1. Periodic Synchronization loop based on appConfig.SyncInterval
	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("[Watcher] Periodic sync triggered")
				refreshConfig()
			case <-ctx.Done():
				return
			}
		}
	}()

	// 2. Hot Reload loop using fsnotify if enabled in config
	if appConfig.HotReload {
		go startHotReloadLoop(ctx)
	}
}

// startHotReloadLoop monitors file events and triggers a reload with debouncing
func startHotReloadLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Watcher] Error: Could not start fsnotify: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the definitions directory recursively
	err = filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		log.Printf("[Watcher] Error: Could not watch directory: %v", err)
	}

	var timer *time.Timer
	debounceDuration := time.Duration(appConfig.HotReloadDebounce) * time.Second

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Monitor write, create and remove events on YAML files
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				if strings.HasSuffix(event.Name, ".yaml") || strings.HasSuffix(event.Name, ".yml") {
					// Debounce logic: reset timer on every new event
					if timer != nil {
						timer.Stop()
					}
					timer = time.AfterFunc(debounceDuration, func() {
						log.Printf("[Watcher] Hot Reload triggered by change in: %s", event.Name)
						refreshConfig()
					})
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[Watcher] fsnotify error: %v", err)
		case <-ctx.Done():
			return
		}
	}
}

// refreshConfig orchestrates the load, validation, and push process
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Configuration loading error: %v", err)
		syncSuccess = false
		return
	}

	configMutex.Lock()
	now := time.Now()
	
	// Apply active downtimes to the current inventory
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
		if hostsInDt[cfg.Services[i].HostName] {
			cfg.Services[i].InDowntime = true
		}
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

	// Push the updated config to the Scheduler
	pushToScheduler(cfg)
}

// pushToScheduler sends the config via POST and logs connection errors
func pushToScheduler(cfg *models.GlobalConfig) {
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("[Watcher] Serialization error: %v", err)
		return
	}

	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	
	// Handle connection issues (unreachable host, timeout, etc.)
	if err != nil {
		log.Printf("[Watcher] CRITICAL: Scheduler unreachable at %s: %v", url, err)
		syncSuccess = false
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		syncSuccess = true
		lastSyncTime = time.Now()
		log.Println("[Watcher] OK: Configuration successfully synced to Scheduler")
	} else {
		log.Printf("[Watcher] WARNING: Sync failed: Scheduler returned HTTP %d", resp.StatusCode)
		syncSuccess = false
	}
}

// loadAndValidateAll reads all YAML files and handles template inheritance
func loadAndValidateAll() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var temp models.GlobalConfig
			if err := yaml.Unmarshal(data, &temp); err != nil {
				log.Printf("[Watcher] YAML Unmarshal error in %s: %v", path, err)
				return nil
			}
			raw.Hosts = append(raw.Hosts, temp.Hosts...)
			raw.Services = append(raw.Services, temp.Services...)
			// Add other fields (Commands, Contacts, etc.) as needed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return processInheritance(raw), nil
}

// Helper to check if a component should be registered in active inventory
func isRegistered(reg *bool) bool {
	return reg == nil || *reg
}

// processInheritance applies 'use' templates to 'register: true' objects
func processInheritance(raw *models.GlobalConfig) *models.GlobalConfig {
	hTpl := make(map[string]models.Host)
	final := &models.GlobalConfig{}

	// 1. Collect templates (register: false)
	for _, h := range raw.Hosts {
		if !isRegistered(h.Register) {
			hTpl[h.ID] = h
		}
	}

	// 2. Build active inventory with inheritance
	for _, h := range raw.Hosts {
		if isRegistered(h.Register) {
			if h.Use != "" {
				if t, ok := hTpl[h.Use]; ok {
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.Address == "" { h.Address = t.Address }
				}
			}
			final.Hosts = append(final.Hosts, h)
		}
	}
	for _, s := range raw.Services {
		if isRegistered(s.Register) {
			final.Services = append(final.Services, s)
		}
	}
	return final
}
