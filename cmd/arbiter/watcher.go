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
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// startWatcher starts the periodic sync and file watching loops.
func startWatcher(ctx context.Context) {
	refreshConfig() // Initial load
	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if isLeader() {
					log.Println("[WATCHER] Periodic sync triggered...")
					refreshConfig()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	if appConfig.HotReload {
		go startHotReloadLoop(ctx)
	}
}

// refreshConfig loads, validates, and pushes the config to the Scheduler with Retries.
func refreshConfig() {
	cfg, err := loadAndProcess()
	if err != nil {
		log.Printf("[ERROR] YAML Processing: %v", err)
		return
	}

	// Audit with Linter
	audit := RunLinter(cfg)
	if len(audit.Errors) > 0 {
		log.Printf("[LINTER] SYNC ABORTED: %d errors found", len(audit.Errors))
		for _, e := range audit.Errors {
			log.Printf(" -> %s", e)
		}
		return
	}

	// Logging detailed counts
	log.Printf("[LINTER] Configuration valid. Objects: H:%d, S:%d, CMD:%d, TP:%d, C:%d, HG:%d, SG:%d",
		audit.Counts.Hosts, audit.Counts.Services, audit.Counts.Commands,
		audit.Counts.TimePeriods, audit.Counts.Contacts, audit.Counts.HostGroups, audit.Counts.ServiceGroups)

	configMutex.Lock()
	cfg.Downtimes = downtimes
	currentConfig = *cfg
	lastSyncTime = time.Now()
	configMutex.Unlock()

	// Push to Scheduler with Exponential Retry
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"

	go func() {
		maxRetries := 3
		wait := 2 * time.Second
		for i := 1; i <= maxRetries; i++ {
			log.Printf("[SCHEDULER] Push attempt %d/%d to %s", i, maxRetries, url)
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
			
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				syncSuccess = true
				log.Printf("[SCHEDULER] SUCCESS: Configuration applied by Scheduler")
				return
			}

			syncSuccess = false
			errMsg := "Connection refused"
			if err != nil {
				errMsg = err.Error()
			} else {
				errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				resp.Body.Close()
			}

			log.Printf("[SCHEDULER] FAILED (Attempt %d): %s", i, errMsg)
			if i < maxRetries {
				log.Printf("[SCHEDULER] Retrying in %v...", wait)
				time.Sleep(wait)
				wait *= 2
			}
		}
		log.Printf("[SCHEDULER] FATAL: Failed to sync after %d attempts", maxRetries)
	}()
}

// loadAndProcess handles file reading and 'use' keyword inheritance.
func loadAndProcess() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			data, err := os.ReadFile(path)
			if err != nil { return err }
			var tmp models.GlobalConfig
			if err := yaml.Unmarshal(data, &tmp); err == nil {
				raw.Hosts = append(raw.Hosts, tmp.Hosts...)
				raw.Services = append(raw.Services, tmp.Services...)
				raw.Commands = append(raw.Commands, tmp.Commands...)
				raw.TimePeriods = append(raw.TimePeriods, tmp.TimePeriods...)
				raw.Contacts = append(raw.Contacts, tmp.Contacts...)
				raw.HostGroups = append(raw.HostGroups, tmp.HostGroups...)
				raw.ServiceGroups = append(raw.ServiceGroups, tmp.ServiceGroups...)
			}
		}
		return nil
	})

	// Process Inheritance
	templates := make(map[string]models.Host)
	final := &models.GlobalConfig{
		Commands: raw.Commands, TimePeriods: raw.TimePeriods, Contacts: raw.Contacts,
		HostGroups: raw.HostGroups, ServiceGroups: raw.ServiceGroups,
	}

	for _, h := range raw.Hosts {
		if h.Register != nil && !*h.Register { templates[h.ID] = h }
	}
	for _, h := range raw.Hosts {
		if h.Register == nil || *h.Register {
			if h.Use != "" {
				if t, ok := templates[h.Use]; ok {
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.CheckPeriod == "" { h.CheckPeriod = t.CheckPeriod }
				}
			}
			final.Hosts = append(final.Hosts, h)
		}
	}
	final.Services = raw.Services
	return final, err
}

func startHotReloadLoop(ctx context.Context) {
	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()
	watcher.Add(appConfig.DefinitionsDir)
	for {
		select {
		case event := <-watcher.Events:
			if isLeader() && event.Op&fsnotify.Write == fsnotify.Write {
				log.Printf("[HOTRELOAD] Change detected: %s", event.Name)
				refreshConfig()
				broadcastToFollowers()
			}
		case <-ctx.Done():
			return
		}
	}
}
