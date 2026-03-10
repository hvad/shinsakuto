package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

var (
	mu sync.RWMutex
)

func main() {
	configPath := flag.String("c", "config.json", "Path to configuration file")
	isDaemon := flag.Bool("d", false, "Run as background daemon")
	flag.Parse()

	// 1. Load component configuration
	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize loggers (System logger via pkg/logger + Alert auditor)
	initLoggers()

	// 3. Handle Background Daemonization
	if *isDaemon && os.Getenv("REACTIONNER_DAEMON") != "true" {
		cmd := exec.Command(os.Args[0], "-config", *configPath)
		cmd.Env = append(os.Environ(), "REACTIONNER_DAEMON=true")
		if err := cmd.Start(); err != nil {
			logger.Fatal("Failed to start daemon: %v", err)
		}
		fmt.Printf("[INFO] Reactionner starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 4. High Availability (Raft) Initialization
	if appConfig.HAEnabled {
		logger.Info("[HA] Initializing Raft cluster node: %s", appConfig.RaftNodeID)
		if err := setupRaft(); err != nil {
			logger.Fatal("Raft setup failed: %v", err)
		}
	}

	// 5. HTTP API Route Registration
	http.HandleFunc("/v1/notify", notifyHandler)
	http.HandleFunc("/v1/ack", ackHandler)
	http.HandleFunc("/v1/status", statusHandler)

	listenAddr := fmt.Sprintf("%s:%d", appConfig.Address, appConfig.Port)
	logger.Always("Shinsakuto Reactionner listening on %s", listenAddr)

	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		logger.Fatal("API server crashed: %v", err)
	}
}

// notifyHandler processes incoming alerts from Schedulers
func notifyHandler(w http.ResponseWriter, r *http.Request) {
	var req models.NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// If it's a RECOVERY and HA is enabled, synchronize state across the cluster
	if req.Type == "RECOVERY" && appConfig.HAEnabled && isLeader() {
		payload, _ := json.Marshal(RaftPayload{Action: "RECOVERY", ID: req.EntityID})
		raftNode.Apply(payload, 5*time.Second)
		logger.Info("[HA] Replicated recovery state for entity: %s", req.EntityID)
	}

	processNotification(req)
	w.WriteHeader(http.StatusOK)
}

// ackHandler manages manual acknowledgments to mute alerts
func ackHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if appConfig.HAEnabled {
		if !isLeader() {
			http.Error(w, "Not the cluster leader", http.StatusForbidden)
			return
		}
		payload, _ := json.Marshal(RaftPayload{Action: "ACK", ID: body.EntityID})
		raftNode.Apply(payload, 5*time.Second)
	} else {
		mu.Lock()
		acknowledgments[body.EntityID] = true
		mu.Unlock()
	}

	logger.Info("[ACK] Entity %s acknowledged by user", body.EntityID)
	w.WriteHeader(http.StatusOK)
}

// statusHandler returns current health and HA role
func statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"is_leader": isLeader(),
		"ha_active": appConfig.HAEnabled,
		"address":   appConfig.Address,
		"port":      appConfig.Port,
		"uptime":    time.Now().Unix(),
	}
	json.NewEncoder(w).Encode(status)
}
