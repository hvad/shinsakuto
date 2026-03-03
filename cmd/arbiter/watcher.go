package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// startWatcher initializes the configuration management loops.
func startWatcher(ctx context.Context) {
	// Initial load of the configuration
	refreshConfig()

	// Periodic synchronization loop
	go func() {
		// Interval based on sync_interval from config.json
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if isLeader() {
					log.Println("[WATCHER] Periodic check: broadcasting configuration to cluster nodes")
					refreshConfig()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Hot Reload loop if enabled
	if appConfig.HotReload {
		go startHotReloadLoop(ctx)
	}
}

// refreshConfig loads YAML files, updates the internal state, and synchronizes the cluster if Leader.
func refreshConfig() {
	// 1. Load and process all YAML definitions from disk
	cfg, err := loadAndProcess()
	if err != nil {
		log.Printf("[ERROR] Failed to load configuration files: %v", err)
		return
	}

	// 2. Update the shared global state protected by Mutex
	configMutex.Lock()
	currentConfig = *cfg
	lastSyncTime = time.Now()
	configMutex.Unlock()

	// 3. Only the Leader (or Solo node) is allowed to push to Scheduler and Followers
	if !isLeader() {
		return
	}

	// 4. Validate configuration integrity with the Linter
	audit := RunLinter(cfg)
	if len(audit.Errors) > 0 {
		log.Printf("[LINTER] Configuration rejected: %d critical errors found", len(audit.Errors))
		syncSuccess = false
		return
	}

	// 5. Push validated configuration to the central Scheduler
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	sendToScheduler(url, data)

	// 6. Propagate configuration files to all HA Follower nodes
	if appConfig.HAEnabled {
		broadcastToFollowers()
	}
}

// loadAndProcess recursively scans the definitions directory and parses YAML files.
func loadAndProcess() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

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

	// Basic Shinken-style inheritance logic for registered objects
	final := &models.GlobalConfig{
		Commands:      raw.Commands,
		TimePeriods:   raw.TimePeriods,
		Contacts:      raw.Contacts,
		HostGroups:    raw.HostGroups,
		ServiceGroups: raw.ServiceGroups,
	}

	for _, h := range raw.Hosts {
		if h.Register == nil || *h.Register {
			final.Hosts = append(final.Hosts, h)
		}
	}
	for _, s := range raw.Services {
		if s.Register == nil || *s.Register {
			final.Services = append(final.Services, s)
		}
	}

	return final, err
}

// sendToScheduler pushes the full configuration JSON to the Scheduler component.
func sendToScheduler(url string, data []byte) {
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[WATCHER] Scheduler unreachable at %s: %v", url, err)
		syncSuccess = false
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Println("[WATCHER] Configuration successfully synchronized with Scheduler")
		syncSuccess = true
	} else {
		log.Printf("[WATCHER] Scheduler returned error status: %d", resp.StatusCode)
		syncSuccess = false
	}
}

// broadcastToFollowers compresses and sends the local definitions to all cluster peers.
func broadcastToFollowers() {
	if !isLeader() {
		return
	}

	// Prepare the TGZ archive of the definitions directory in memory
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(appConfig.DefinitionsDir, path)
		header, _ := tar.FileInfoHeader(info, "")
		header.Name = relPath
		tw.WriteHeader(header)
		f, _ := os.Open(path)
		io.Copy(tw, f)
		f.Close()
		return nil
	})

	tw.Close()
	gzw.Close()

	// Broadcast to all peers except self
	for _, nodeAddr := range appConfig.ClusterNodes {
		// Avoid sending sync to current node's API address
		selfAddr := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)
		if nodeAddr == selfAddr {
			continue
		}

		go func(addr string, payload []byte) {
			url := fmt.Sprintf("http://%s/v1/cluster/sync-receiver", addr)
			resp, err := httpClient.Post(url, "application/x-gzip", bytes.NewReader(payload))
			if err != nil {
				log.Printf("[HA] Failed to propagate files to follower %s: %v", addr, err)
				return
			}
			resp.Body.Close()
			log.Printf("[HA] Configuration successfully propagated to follower %s", addr)
		}(nodeAddr, buf.Bytes())
	}
}

// startHotReloadLoop watches the file system for changes to trigger an immediate sync.
func startHotReloadLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[ERROR] Failed to initialize FS watcher: %v", err)
		return
	}
	defer watcher.Close()

	watcher.Add(appConfig.DefinitionsDir)
	log.Printf("[WATCHER] HotReload enabled. Monitoring: %s", appConfig.DefinitionsDir)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Trigger refresh on file creation or modification
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if isLeader() {
					log.Printf("[HOTRELOAD] Change detected in %s, triggering sync", event.Name)
					refreshConfig()
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
