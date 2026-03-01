package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"shinsakuto/pkg/models"
)

func startAPI() {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		configMutex.RLock()
		defer configMutex.RUnlock()
		role := "Standalone"
		if appConfig.HAEnabled && raftNode != nil {
			role = raftNode.State().String()
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node":      appConfig.RaftNodeID,
			"role":      role,
			"last_sync": lastSyncTime.Format(time.RFC3339),
			"synced":    syncSuccess,
		})
	})

	mux.HandleFunc("/v1/downtime", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if !isLeader() {
				http.Error(w, "Not leader", 403)
				return
			}
			var d models.Downtime
			json.NewDecoder(r.Body).Decode(&d)
			d.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())
			
			if appConfig.HAEnabled && raftNode != nil {
				payload, _ := json.Marshal(LogPayload{Action: "ADD_DT", Data: d})
				raftNode.Apply(payload, 5*time.Second)
			} else {
				configMutex.Lock()
				downtimes = append(downtimes, d)
				configMutex.Unlock()
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(d)
			go refreshConfig()
			return
		}
		json.NewEncoder(w).Encode(downtimes)
	})

	mux.HandleFunc("/v1/cluster/sync-receiver", func(w http.ResponseWriter, r *http.Request) {
		if isLeader() || !appConfig.HAEnabled { return }
		gzr, _ := gzip.NewReader(r.Body)
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF { break }
			target := filepath.Join(appConfig.DefinitionsDir, hdr.Name)
			os.MkdirAll(filepath.Dir(target), 0755)
			f, _ := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
			io.Copy(f, tr)
			f.Close()
		}
		w.WriteHeader(200)
		go refreshConfig()
	})

	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)
	log.Printf("[API] Running on %s", addr)
	http.ListenAndServe(addr, mux)
}
