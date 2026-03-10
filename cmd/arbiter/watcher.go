package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

var (
	httpClient        = &http.Client{Timeout: 15 * time.Second}
	isInCoolOff       = false
	coolOffStartTime  time.Time
)

// startWatcher begins the configuration monitoring and synchronization loops.
func startWatcher(ctx context.Context) {
	refreshConfig()

	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if isLeader() {
					logger.Info("[WATCHER] Periodic sharding sync triggered by leader")
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

// refreshConfig reloads definitions from disk, audits them, and syncs to Schedulers.
func refreshConfig() {
	if isInCoolOff {
		coolOffDuration := time.Duration(appConfig.SchedulerCoolOffMinutes) * time.Minute
		if time.Since(coolOffStartTime) < coolOffDuration {
			logger.Info("[WATCHER] Sync skipped: system in cool-off until %v", coolOffStartTime.Add(coolOffDuration).Format("15:04:05"))
			return
		}
		isInCoolOff = false 
	}

	cfg, err := loadAndProcess()
	if err != nil {
		logger.Always("[ERROR] Failed to process configuration: %v", err)
		return
	}

	configMutex.Lock()
	currentConfig = *cfg
	lastSyncTime = time.Now()
	configMutex.Unlock()

	if !isLeader() {
		return
	}

	audit := RunLinter(cfg)
	if len(audit.Errors) > 0 {
		logger.Always("[LINTER] Rejected: %d errors found", len(audit.Errors))
		syncSuccess = false
		return
	}

	shards := partitionConfig(cfg, len(appConfig.SchedulerURLs))
	go syncShardsToSchedulers(shards)

	if appConfig.HAEnabled {
		broadcastToFollowers()
	}
}

// partitionConfig splits the global configuration into multiple shards for Schedulers.
func partitionConfig(fullCfg *models.GlobalConfig, n int) []models.GlobalConfig {
	if n <= 1 {
		return []models.GlobalConfig{*fullCfg}
	}

	shards := make([]models.GlobalConfig, n)
	for i := 0; i < n; i++ {
		shards[i] = models.GlobalConfig{
			Commands:    fullCfg.Commands,
			TimePeriods: fullCfg.TimePeriods,
			Contacts:    fullCfg.Contacts,
			Hosts:       []models.Host{},
			Services:    []models.Service{},
		}
	}

	hostToShard := make(map[string]int)
	for i, host := range fullCfg.Hosts {
		shardIdx := i % n
		shards[shardIdx].Hosts = append(shards[shardIdx].Hosts, host)
		hostToShard[host.ID] = shardIdx
	}

	for _, service := range fullCfg.Services {
		if idx, ok := hostToShard[service.HostName]; ok {
			shards[idx].Services = append(shards[idx].Services, service)
		}
	}

	return shards
}

// syncShardsToSchedulers sends partitioned configurations to their respective Scheduler nodes.
func syncShardsToSchedulers(shards []models.GlobalConfig) {
	successCount := 0
	totalSchedulers := len(appConfig.SchedulerURLs)

	if totalSchedulers == 0 {
		logger.Always("[WARNING] No Scheduler URLs configured")
		return
	}

	for i, rawURL := range appConfig.SchedulerURLs {
		if i >= len(shards) { break }
		
		url := strings.TrimSuffix(rawURL, "/") + "/v1/sync-all"
		data, _ := json.Marshal(shards[i])
		
		for attempt := 1; attempt <= 3; attempt++ {
			logger.Info("[WATCHER] Sending shard %d to %s (Attempt %d/3)", i, url, attempt)
			
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				logger.Info("[WATCHER] Successfully synchronized shard %d with %s", i, url)
				successCount++
				break 
			}

			if err != nil {
				logger.Info("[WATCHER] Attempt %d failed for %s: %v", attempt, url, err)
			} else {
				logger.Info("[WATCHER] Attempt %d failed for %s: Status %d", attempt, url, resp.StatusCode)
				resp.Body.Close()
			}

			if attempt < 3 {
				time.Sleep(5 * time.Second) 
			}
		}
	}

	if successCount == 0 {
		logger.Info("[CRITICAL] Failed to reach any Scheduler. Entering cool-off for %d minutes", appConfig.SchedulerCoolOffMinutes)
		isInCoolOff = true
		coolOffStartTime = time.Now()
		syncSuccess = false
	} else {
		syncSuccess = (successCount == totalSchedulers)
	}
}

// loadAndProcess handles the inheritance resolution and command linkage.
func loadAndProcess() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	
	// 1. Traverse and Unmarshal all YAML definitions
	err := filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return err }
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
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
	if err != nil { return nil, err }

	// 2. Map Commands for quick lookup during linkage
	commandMap := make(map[string]models.Command)
	for _, cmd := range raw.Commands {
		commandMap[cmd.ID] = cmd
	}

	final := &models.GlobalConfig{
		Commands:    raw.Commands,
		TimePeriods: raw.TimePeriods,
		Contacts:    raw.Contacts,
	}

	// 3. Process Templates
	hTemplates := make(map[string]models.Host)
	for _, h := range raw.Hosts {
		if h.Register != nil && !*h.Register { hTemplates[h.ID] = h }
	}
	sTemplates := make(map[string]models.Service)
	for _, s := range raw.Services {
		if s.Register != nil && !*s.Register { sTemplates[s.ID] = s }
	}

	hGroups := make(map[string]*models.HostGroup)
	for i := range raw.HostGroups { hGroups[raw.HostGroups[i].ID] = &raw.HostGroups[i] }
	
	sGroups := make(map[string]*models.ServiceGroup)
	for i := range raw.ServiceGroups { sGroups[raw.ServiceGroups[i].ID] = &raw.ServiceGroups[i] }

	// 4. Resolve Host Inheritance
	for _, h := range raw.Hosts {
		if h.Register == nil || *h.Register {
			resolved := resolveHostInheritance(h, hTemplates, 0)
			final.Hosts = append(final.Hosts, resolved)
			for _, gn := range resolved.HostGroups {
				if group, ok := hGroups[gn]; ok {
					group.Members = append(group.Members, resolved.ID)
				} else {
					hGroups[gn] = &models.HostGroup{ID: gn, Members: []string{resolved.ID}}
				}
			}
		}
	}

	// 5. Resolve Service Inheritance and LINK Commands
	for _, s := range raw.Services {
		if s.Register == nil || *s.Register {
			resolved := resolveServiceInheritance(s, sTemplates, 0)

			// LINKING PHASE: Bind the Command object to the Service
			if cmd, exists := commandMap[resolved.CheckCommand]; exists {
				resolved.CommandDefinition = cmd
			} else {
				// Fallback for raw inline commands like "echo 'Test'"
				resolved.CommandDefinition = models.Command{
					ID:          "inline_exec",
					CommandLine: resolved.CheckCommand,
				}
			}

			final.Services = append(final.Services, resolved)
			for _, gn := range resolved.ServiceGroups {
				if group, ok := sGroups[gn]; ok {
					group.Members = append(group.Members, fmt.Sprintf("%s,%s", resolved.HostName, resolved.ID))
				} else {
					sGroups[gn] = &models.ServiceGroup{ID: gn, Members: []string{fmt.Sprintf("%s,%s", resolved.HostName, resolved.ID)}}
				}
			}
		}
	}

	for _, g := range hGroups { final.HostGroups = append(final.HostGroups, *g) }
	for _, g := range sGroups { final.ServiceGroups = append(final.ServiceGroups, *g) }

	return final, nil
}

// resolveHostInheritance recurses through "use" fields to build a complete Host object.
func resolveHostInheritance(h models.Host, templates map[string]models.Host, depth int) models.Host {
	if depth > 5 || h.Use == "" { return h }
	if parent, ok := templates[h.Use]; ok {
		p := resolveHostInheritance(parent, templates, depth+1)
		if h.Address == "" { h.Address = p.Address }
		if h.CheckCommand == "" { h.CheckCommand = p.CheckCommand }
		if h.CheckPeriod == "" { h.CheckPeriod = p.CheckPeriod }
		if len(h.Contacts) == 0 { h.Contacts = p.Contacts }
		if len(h.HostGroups) == 0 { h.HostGroups = p.HostGroups }
	}
	return h
}

// resolveServiceInheritance recurses through "use" fields to build a complete Service object.
func resolveServiceInheritance(s models.Service, templates map[string]models.Service, depth int) models.Service {
	if depth > 5 || s.Use == "" { return s }
	if parent, ok := templates[s.Use]; ok {
		p := resolveServiceInheritance(parent, templates, depth+1)
		if s.CheckCommand == "" { s.CheckCommand = p.CheckCommand }
		if s.CheckPeriod == "" { s.CheckPeriod = p.CheckPeriod }
		if len(s.Contacts) == 0 { s.Contacts = p.Contacts }
		if len(s.ServiceGroups) == 0 { s.ServiceGroups = p.ServiceGroups }
	}
	return s
}

// broadcastToFollowers propagates local definitions to all Raft followers.
func broadcastToFollowers() {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return nil }
		rel, _ := filepath.Rel(appConfig.DefinitionsDir, path)
		header, _ := tar.FileInfoHeader(info, "")
		header.Name = rel
		tw.WriteHeader(header)
		f, _ := os.Open(path)
		io.Copy(tw, f)
		f.Close()
		return nil
	})
	tw.Close(); gzw.Close()

	payload := buf.Bytes()
	self := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)

	for _, addr := range appConfig.ClusterNodes {
		if addr == self { continue }
		go func(target string) {
			url := fmt.Sprintf("http://%s/v1/cluster/sync-receiver", target)
			resp, err := httpClient.Post(url, "application/x-gzip", bytes.NewReader(payload))
			if err == nil {
				resp.Body.Close()
				logger.Info("[HA] Propagated config to follower: %s", target)
			}
		}(addr)
	}
}

// startHotReloadLoop watches for file changes to trigger automatic re-syncing.
func startHotReloadLoop(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil { return }
	defer w.Close()
	w.Add(appConfig.DefinitionsDir)
	for {
		select {
		case ev := <-w.Events:
			if (ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create) && isLeader() {
				logger.Always("[HOTRELOAD] Change detected in %s, re-syncing shards", ev.Name)
				refreshConfig()
			}
		case <-ctx.Done():
			return
		}
	}
}
