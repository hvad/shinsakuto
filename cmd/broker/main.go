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
)

// dataQueue is the internal asynchronous buffer
var dataQueue = make(chan models.CheckResult, 50000)

func main() {
	configPath := flag.String("config", "broker.json", "Path to broker configuration file")
	daemonMode := flag.Bool("d", false, "Run as a background daemon")
	flag.Parse()

	// Load configuration
	if err := loadConfig(*configPath); err != nil {
		log.Fatalf("Fatal: Could not load config: %v", err)
	}
	initLogger()

	// Handle Linux Daemonization
	if *daemonMode {
		args := os.Args[1:]
		newArgs := make([]string, 0)
		for _, arg := range args {
			if arg != "-d" {
				newArgs = append(newArgs, arg)
			}
		}
		cmd := exec.Command(os.Args[0], newArgs...)
		if err := cmd.Start(); err != nil {
			fmt.Printf("Error: Failed to daemonize: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[INFO] Shinsakuto Broker starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// Start database worker pool
	startWorkers(dataQueue)

	// API Route for Schedulers to push data
	http.HandleFunc("/v1/broker/data", handleIngestion)

	log.Printf("[INFO] Shinsakuto Broker listening on port %d", appConfig.APIPort)
	serverAddr := fmt.Sprintf(":%d", appConfig.APIPort)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Fatal: API server failed: %v", err)
	}
}

// handleIngestion decodes the result and puts it in the async queue
func handleIngestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	var res models.CheckResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Non-blocking write to the queue
	select {
	case dataQueue <- res:
		w.WriteHeader(http.StatusAccepted)
	default:
		log.Printf("[WARN] Broker queue full! Dropping data for %s", res.ID)
		http.Error(w, "Broker overloaded", http.StatusServiceUnavailable)
	}
}
