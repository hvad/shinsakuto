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

// startWatcher initializes the background loops for configuration management.
func startWatcher(ctx context.Context) {
	// Initial configuration load
	refreshConfig()

	// Periodic synchronization loop to maintain cluster consistency
	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if isLeader() {
					log.Println("[WATCHER] Periodic cluster-wide configuration sync triggered by leader")
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

// refreshConfig triggers a full reload, validation, and propagation cycle.
func refreshConfig() {
	cfg, err := loadAndProcess()
	if err != nil {
		log.Printf("[ERROR] Failed to process configuration: %v", err)
		return
	}

	configMutex.Lock()
	currentConfig = *cfg
	lastSyncTime = time.Now()
	configMutex.Unlock()

	// Only the Leader is allowed to push to Scheduler and Followers
	if !isLeader() {
		return
	}

	// Validate configuration integrity
	audit := RunLinter(cfg)
	if len(audit.Errors) > 0 {
		log.Printf("[LINTER] Rejected: %d errors found", len(audit.Errors))
		syncSuccess = false
		return
	}

	// Push validated configuration to the central Scheduler
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	sendToScheduler(url, data)

	// Propagate configuration files to all HA Follower nodes
	if appConfig.HAEnabled {
		broadcastToFollowers()
	}
}

// loadAndProcess handles recursive inheritance and automatic group assignment.
func loadAndProcess() (*models.GlobalConfig, error) {
	raw := &models.GlobalConfig{}
	
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

	final := &models.GlobalConfig{
		Commands:    raw.Commands,
		TimePeriods: raw.TimePeriods,
		Contacts:    raw.Contacts,
	}

	// Index templates for quick lookup
	hTemplates := make(map[string]models.Host)
	for _, h := range raw.Hosts {
		if h.Register != nil && !*h.Register { hTemplates[h.ID] = h }
	}
	sTemplates := make(map[string]models.Service)
	for _, s := range raw.Services {
		if s.Register != nil && !*s.Register { sTemplates[s.ID] = s }
	}

	// Dynamic group indexing
	hGroups := make(map[string]*models.HostGroup)
	for i := range raw.HostGroups { hGroups[raw.HostGroups[i].ID] = &raw.HostGroups[i] }
	
	sGroups := make(map[string]*models.ServiceGroup)
	for i := range raw.ServiceGroups { sGroups[raw.ServiceGroups[i].ID] = &raw.ServiceGroups[i] }

	// Process Hosts with inheritance and auto-grouping
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

	// Process Services with inheritance and auto-grouping
	for _, s := range raw.Services {
		if s.Register == nil || *s.Register {
			resolved := resolveServiceInheritance(s, sTemplates, 0)
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

	// Reassemble groups into final config
	for _, g := range hGroups { final.HostGroups = append(final.HostGroups, *g) }
	for _, g := range sGroups { final.ServiceGroups = append(final.ServiceGroups, *g) }

	return final, nil
}

// resolveHostInheritance merges host properties recursively from templates.
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

// resolveServiceInheritance merges service properties recursively from templates.
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

// broadcastToFollowers compresses and propagates definitions to all peer nodes.
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
				log.Printf("[HA] Propagated config to follower: %s", target)
			}
		}(addr)
	}
}

// sendToScheduler pushes configuration to the central Scheduler component.
func sendToScheduler(url string, data []byte) {
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[WATCHER] Scheduler connection failed: %v", err)
		syncSuccess = false
		return
	}
	defer resp.Body.Close()
	syncSuccess = (resp.StatusCode == http.StatusOK)
}

// startHotReloadLoop watches the filesystem for changes.
func startHotReloadLoop(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil { return }
	defer w.Close()
	w.Add(appConfig.DefinitionsDir)
	for {
		select {
		case ev := <-w.Events:
			if (ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create) && isLeader() {
				log.Printf("[HOTRELOAD] Change detected in %s, re-syncing cluster", ev.Name)
				refreshConfig()
			}
		case <-ctx.Done():
			return
		}
	}
}
