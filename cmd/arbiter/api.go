package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shinsakuto/pkg/models"
	"github.com/hashicorp/raft"
)

// startAPI initializes the HTTP server for administration, metrics, and cluster coordination.
func startAPI() {
	mux := http.NewServeMux()

	// Monitoring & Health
	mux.HandleFunc("/v1/metrics", handleMetrics)
	mux.HandleFunc("/v1/status", handleStatus)

	// Business Logic
	mux.HandleFunc("/v1/downtime", handleDowntime)

	// Cluster & HA
	mux.HandleFunc("/v1/cluster/sync-receiver", handleClusterSync)
	mux.HandleFunc("/v1/cluster/join", handleJoin)

	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)
	logArbiter("[API] Arbiter API server listening on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		logFatal("[FATAL] Arbiter API server failed: %v", err)
	}
}

// handleMetrics serves hardware and business metrics in Prometheus-style format.
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	stats := getSystemMetrics()

	fmt.Fprintf(w, "# Shinsakuto Arbiter Internal Metrics\n")
	fmt.Fprintf(w, "arbiter_cpu_user_seconds_total %.2f\n", stats.CPUUser)
	fmt.Fprintf(w, "arbiter_cpu_system_seconds_total %.2f\n", stats.CPUSys)
	fmt.Fprintf(w, "arbiter_process_uptime_seconds %.0f\n", time.Since(startTime).Seconds())
	fmt.Fprintf(w, "arbiter_mem_alloc_bytes %d\n", stats.MemAlloc)
	fmt.Fprintf(w, "arbiter_mem_rss_bytes %d\n", stats.MemRSS)
	fmt.Fprintf(w, "arbiter_goroutines_count %d\n", stats.Goroutines)

	configMutex.RLock()
	fmt.Fprintf(w, "arbiter_monitored_hosts_total %d\n", len(currentConfig.Hosts))
	fmt.Fprintf(w, "arbiter_monitored_services_total %d\n", len(currentConfig.Services))
	configMutex.RUnlock()
}

// handleStatus returns JSON information about the node's current sharding state.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()

	role := "Standalone"
	if appConfig.HAEnabled && raftNode != nil {
		role = raftNode.State().String()
	}

	res := map[string]interface{}{
		"node_id":         appConfig.RaftNodeID,
		"role":            role,
		"last_sync":       lastSyncTime.Format(time.RFC3339),
		"sync_ok":         syncSuccess,
		"scheduler_count": len(appConfig.SchedulerURLs),
		"is_sharded":      len(appConfig.SchedulerURLs) > 1,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// handleDowntime manages the registration of maintenance windows.
func handleDowntime(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !isLeader() {
			logArbiter("[WARNING] Forbidden: Downtime request rejected, not the Raft Leader")
			http.Error(w, "Forbidden: Not Raft Leader", http.StatusForbidden)
			return
		}

		var d models.Downtime
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, "[WARNING] Invalid JSON payload", http.StatusBadRequest)
			return
		}

		d.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())

		if appConfig.HAEnabled && raftNode != nil {
			payload, _ := json.Marshal(LogPayload{Action: "ADD_DT", Data: d})
			raftNode.Apply(payload, 5*time.Second)
		} else {
			configMutex.Lock()
			downtimes = append(downtimes, d)
			configMutex.Unlock()
		}

		logArbiter("[API] New downtime registered: %s", d.ID)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(d)
		go refreshConfig()
		return
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(downtimes)
}

// handleClusterSync receives a TGZ archive from the Leader and extracts it locally.
func handleClusterSync(w http.ResponseWriter, r *http.Request) {
	if isLeader() || !appConfig.HAEnabled {
		logArbiter("[HA] Cluster sync rejected: node is leader or HA disabled")
		http.Error(w, "[WARNING] Rejected", http.StatusForbidden)
		return
	}

	logArbiter("[HA] Receiving cluster configuration sync...")
	gzr, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, "[WARNING] Invalid GZIP", http.StatusBadRequest)
		return
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF { break }
		if err != nil { break }

		target := filepath.Join(appConfig.DefinitionsDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, _ := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if f != nil {
				io.Copy(f, tr)
				f.Close()
			}
		}
	}
	w.WriteHeader(http.StatusOK)
	logArbiter("[HA] Cluster configuration sync completed")
	go refreshConfig()
}

// handleJoin processes requests from new nodes wanting to join the Raft cluster.
func handleJoin(w http.ResponseWriter, r *http.Request) {
	if !isLeader() {
		http.Error(w, "Not leader", http.StatusTemporaryRedirect)
		return
	}

	var req struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	logArbiter("[HA] Node %s (addr: %s) joining Raft cluster", req.NodeID, req.Address)
	future := raftNode.AddVoter(raft.ServerID(req.NodeID), raft.ServerAddress(req.Address), 0, 0)
	if err := future.Error(); err != nil {
		logArbiter("[ERROR] Failed to add voter %s: %v", req.NodeID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
