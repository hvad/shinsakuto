package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"shinsakuto/pkg/models"
	"sync"
	"time"
)

var (
	mu sync.RWMutex
)

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	isDaemon := flag.Bool("d", false, "Run as background daemon")
	flag.Parse()

	if err := loadConfig(*configPath); err != nil {
		log.Fatalf("Fatal: Failed to load configuration: %v", err)
	}

	initLoggers()

	if *isDaemon && os.Getenv("REACTIONNER_DAEMON") != "true" {
		cmd := exec.Command(os.Args[0], "-config", *configPath)
		cmd.Env = append(os.Environ(), "REACTIONNER_DAEMON=true")
		cmd.Start()
		fmt.Printf("[INFO] Reactionner starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	if appConfig.HAEnabled {
		if err := setupRaft(); err != nil {
			systemLogger.Fatalf("Raft setup failed: %v", err)
		}
	}

	// API Handlers
	http.HandleFunc("/v1/notify", notifyHandler)
	http.HandleFunc("/v1/ack", ackHandler)
	http.HandleFunc("/v1/status", statusHandler)

	systemLogger.Printf("[INFO] Reactionner ready on port %d", appConfig.APIPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", appConfig.APIPort), nil))
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {
	var req models.NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad JSON", 400)
		return
	}
	
	// Replicate state change via Raft if recovery
	if req.Type == "RECOVERY" && appConfig.HAEnabled && isLeader() {
		payload, _ := json.Marshal(RaftPayload{Action: "RECOVERY", ID: req.EntityID})
		raftNode.Apply(payload, 5*time.Second)
	}
	
	processNotification(req)
	w.WriteHeader(http.StatusOK)
}

func ackHandler(w http.ResponseWriter, r *http.Request) {
	var body struct{ EntityID string `json:"entity_id"` }
	json.NewDecoder(r.Body).Decode(&body)
	
	if appConfig.HAEnabled {
		if !isLeader() { http.Error(w, "Not leader", 403); return }
		payload, _ := json.Marshal(RaftPayload{Action: "ACK", ID: body.EntityID})
		raftNode.Apply(payload, 5*time.Second)
	} else {
		mu.Lock()
		acknowledgments[body.EntityID] = true
		mu.Unlock()
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	role := "solo"
	if appConfig.HAEnabled && raftNode != nil { role = raftNode.State().String() }
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"role": role, 
		"acks": acknowledgments, 
		"maintenances": maintenances,
	})
}
