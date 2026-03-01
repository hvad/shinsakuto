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

func startWatcher(ctx context.Context) {
	refreshConfig()

	go func() {
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if isLeader() {
					log.Println("[WATCHER] Cycle de synchronisation périodique...")
					refreshConfig()
					
					log.Println("[WATCHER] Synchronisation de la configuration de supervision...")
					broadcastToFollowers()
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

//func refreshConfig() {
//	cfg, err := loadAndProcess()
//	if err != nil {
//		log.Printf("[ERREUR] %v", err)
//		return
//	}
//
//	audit := RunLinter(cfg)
//	if len(audit.Errors) > 0 {
//		log.Printf("[LINTER] SYNCHRO REJETÉE : %d erreurs critiques", len(audit.Errors))
//		return
//	}
//
//	configMutex.Lock()
//	currentConfig = *cfg
//	lastSyncTime = time.Now()
//	configMutex.Unlock()
//
//	data, _ := json.Marshal(cfg)
//	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
//	
//	// Journalisation et envoi
//	sendToScheduler(url, data, audit.Counts)
//}

// Dans watcher.go

func refreshConfig() {
	// 1. Chargement et traitement des fichiers (commun à tous)
	cfg, err := loadAndProcess()
	if err != nil {
		log.Printf("[ERREUR] %v", err)
		return
	}

	// 2. Mise à jour de l'état interne
	configMutex.Lock()
	currentConfig = *cfg
	lastSyncTime = time.Now()
	configMutex.Unlock()

	// 3. CONDITION DE SÉCURITÉ MISE À JOUR
	// On n'envoie au scheduler QUE si on est en solo OU si on est le leader HA
	canSend := !appConfig.HAEnabled || isLeader()

	if !canSend {
		// Si on est un Follower, on s'arrête ici
		return 
	}

	// 4. Envoi au Scheduler (uniquement pour Solo ou Leader)
	audit := RunLinter(cfg)
	data, _ := json.Marshal(cfg)
	url := strings.TrimSuffix(appConfig.SchedulerURL, "/") + "/v1/sync-all"
	
	sendToScheduler(url, data, audit.Counts)

	// 5. Si on est Leader HA, on propage aussi aux followers
	if appConfig.HAEnabled && isLeader() {
		broadcastToFollowers()
	}
}

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

	final := &models.GlobalConfig{
		Commands:      raw.Commands,
		TimePeriods:   raw.TimePeriods,
		Contacts:      raw.Contacts,
		HostGroups:    raw.HostGroups,
		ServiceGroups: raw.ServiceGroups,
	}

	hTemplates := make(map[string]models.Host)
	for _, h := range raw.Hosts {
		if h.Register != nil && !*h.Register { hTemplates[h.ID] = h }
	}

	for _, h := range raw.Hosts {
		if h.Register == nil || *h.Register {
			if h.Use != "" {
				if t, ok := hTemplates[h.Use]; ok {
					if h.Address == "" { h.Address = t.Address }
					if h.CheckCommand == "" { h.CheckCommand = t.CheckCommand }
					if h.CheckPeriod == "" { h.CheckPeriod = t.CheckPeriod }
				}
			}
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

func sendToScheduler(url string, data []byte, counts ObjectCounts) {
	log.Printf("[WATCHER] Envoi au Scheduler (%d hôtes, %d services)...", counts.Hosts, counts.Services)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		syncSuccess = false
		log.Printf("[WATCHER] ERREUR : Impossible de joindre le Scheduler : %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		syncSuccess = true
		log.Printf("[WATCHER] SUCCÈS : Configuration synchronisée avec succès")
	} else {
		syncSuccess = false
		log.Printf("[WATCHER] ÉCHEC : Le Scheduler a répondu avec le code %d", resp.StatusCode)
	}
}

func startHotReloadLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil { return }
	defer watcher.Close()
	watcher.Add(appConfig.DefinitionsDir)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok { return }
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if isLeader() {
					log.Printf("[HOTRELOAD] Fichier modifié : %s", event.Name)
					refreshConfig()
					broadcastToFollowers()
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func broadcastToFollowers() {
	if !appConfig.HAEnabled || !isLeader() { return }

	for _, nodeAddr := range appConfig.ClusterNodes {
		if strings.Contains(nodeAddr, appConfig.APIAddress) && strings.Contains(nodeAddr, fmt.Sprintf("%d", appConfig.APIPort)) {
			continue
		}

		go func(addr string) {
			var buf bytes.Buffer
			gzw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gzw)

			filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() { return nil }
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

			url := fmt.Sprintf("http://%s/v1/cluster/sync-receiver", addr)
			resp, err := httpClient.Post(url, "application/x-gzip", &buf)
			if err != nil {
				log.Printf("[HA] Erreur synchro vers %s : %v", addr, err)
				return
			}
			resp.Body.Close()
			log.Printf("[HA] Propagation des fichiers vers %s réussie", addr)
		}(nodeAddr)
	}
}
