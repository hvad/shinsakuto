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

// httpClient is used for all communications with the Scheduler.
// Timeout is crucial to prevent the Arbiter from hanging if the network is slow.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// startWatcher initializes the background routines for configuration management.
func startWatcher(ctx context.Context) {
	// Initial load and sync on startup
	refreshConfig()

	// 1. Periodic Synchronization Loop
	// This ensures the Scheduler stays in sync even if file events were missed.
	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("[Watcher] Triggering scheduled periodic sync...")
				refreshConfig()
			case <-ctx.Done():
				return
			}
		}
	}()

	// 2. Hot Reload Loop (Real-time file monitoring)
	if appConfig.HotReload {
		go startHotReloadLoop(ctx)
	}
}

// startHotReloadLoop watches the filesystem for changes in the definitions directory.
func startHotReloadLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Watcher] CRITICAL: Failed to initialize fsnotify: %v", err)
		return
	}
	defer watcher.Close()

	// Register the directory and all subdirectories to the watcher
	err = filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})

	log.Printf("[Watcher] Hot Reload active. Monitoring: %s", appConfig.DefinitionsDir)

	var timer *time.Timer
	debounce := time.Duration(appConfig.HotReloadDebounce) * time.Second

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok { return }
			// We react to Write, Create, and Remove events on YAML files
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				if strings.HasSuffix(event.Name, ".yaml") || strings.HasSuffix(event.Name, ".yml") {
					// Debouncing: We wait for the filesystem to "settle" before reloading
					if timer != nil { timer.Stop() }
					timer = time.AfterFunc(debounce, func() {
						log.Printf("[Watcher] Change detected: %s. Reloading configuration...", event.Name)
						refreshConfig()
					})
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok { return }
			log.Printf("[Watcher] Filesystem error: %v", err)
		case <-ctx.Done():
			return
		}
	}
}

// refreshConfig coordinates the full pipeline: Load -> Validate -> Lint -> Sync.
func refreshConfig() {
	cfg, err := loadAndValidateAll()
	if err != nil {
		log.Printf("[Watcher] Refresh aborted: Configuration error: %v", err)
		syncSuccess = false
		return
	}

	// Integrated Linter: Check for best practices before syncing
	runLinter(cfg)

	// Thread-safe update of the current configuration
	configMutex.Lock()
	now := time.Now()
	
	// Map active downtimes to objects (Host-level downtime impacts all its services)
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
	}

	cfg.Downtimes = downtimes
	currentConfig = *cfg
	configMutex.Unlock()

	// Final step: Push data to the Scheduler
	pushToScheduler(cfg)
}

// runLinter analyzes the configuration for non-critical best practice violations.
func runLinter(cfg *models.GlobalConfig) {
	log.Println("[Linter] Running best practices check...")
	
	serviceStats := make(map[string]int)
	for _, s := range cfg.Services {
		serviceStats[s.HostName]++
	}

	for _, h := range cfg.Hosts {
		// Rule: Connectivity basics
		if h.Address == "" {
			log.Printf("[Linter] ADVICE: Host '%s' is missing an 'address' field.", h.ID)
		}

		// Rule: Alerting basics
		if len(h.Contacts) == 0 {
			log.Printf("[Linter] ADVICE: Host '%s' has no contacts defined; alerts will be suppressed.", h.ID)
		}
	}
}

// pushToScheduler sends the JSON payload to the remote Scheduler and handles connection errors.
func pushToScheduler(cfg *models.GlobalConfig) {
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		// This log is crucial for production troubleshooting
		log.Printf("[Watcher] CRITICAL: Scheduler UNREACHABLE at %s: %v", url, err)
		syncSuccess = false
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		syncSuccess = true
		lastSyncTime = time.Now()
		log.Println("[Watcher] OK: Sync completed successfully.")
	} else {
		log.Printf("[Watcher] WARNING: Sync failed: Scheduler returned status code %d", resp.StatusCode)
		syncSuccess = false
	}
}

// loadAndValidateAll crawls the directory to build the GlobalConfig object.
func loadAndValidateAll() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			data, _ := os.ReadFile(path)
			var temp models.GlobalConfig
			if err := yaml.Unmarshal(data, &temp); err != nil {
				log.Printf("[Watcher] Skipping invalid YAML file %s: %v", path, err)
				return nil
			}
			raw.Hosts = append(raw.Hosts, temp.Hosts...)
			raw.Services = append(raw.Services, temp.Services...)
			raw.Commands = append(raw.Commands, temp.Commands...)
			raw.Contacts = append(raw.Contacts, temp.Contacts...)
			raw.TimePeriods = append(raw.TimePeriods, temp.TimePeriods...)
		}
		return nil
	})
	if err != nil { return nil, err }
	return processInheritance(raw), nil
}

// processInheritance merges template values into active objects.
func processInheritance(raw *models.GlobalConfig) *models.GlobalConfig {
	hTpl := make(map[string]models.Host)
	final := &models.GlobalConfig{
		Commands: raw.Commands, Contacts: raw.Contacts, TimePeriods: raw.TimePeriods,
	}

	// Separate Templates from Active Objects
	for _, h := range raw.Hosts {
		if h.Register != nil && !*h.Register { hTpl[h.ID] = h }
	}

	// Merge templates into registered hosts
	for _, h := range raw.Hosts {
		if h.Register == nil || *h.Register {
			if h.Use != "" {
				if t, ok := hTpl[h.Use]; ok {
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.CheckPeriod == "" { h.CheckPeriod = t.CheckPeriod }
					if len(h.Contacts) == 0 { h.Contacts = t.Contacts }
				}
			}
			final.Hosts = append(final.Hosts, h)
		}
	}

	for _, s := range raw.Services {
		if s.Register == nil || *s.Register { final.Services = append(final.Services, s) }
	}

	return final
}
