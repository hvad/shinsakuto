package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"shinsakuto/pkg/models"
	"shinsakuto/pkg/logger"
)

// dataQueue is the internal asynchronous buffer for incoming check results
var dataQueue = make(chan models.CheckResult, 50000)

func main() {
	configPath := flag.String("c", "config.json", "Path to broker configuration file")
	daemonMode := flag.Bool("d", false, "Run as a background daemon")
	flag.Parse()

	// 1. Load configuration
	if err := loadConfig(*configPath); err != nil {
		// Use standard fmt because logger is not initialized yet
		fmt.Fprintf(os.Stderr, "Fatal: Could not load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize the centralized logger
	initLogger()

	// 3. Handle Linux Daemonization
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
			logger.Fatal("Failed to daemonize process: %v", err)
		}
		// Feedback is printed to terminal before exit
		fmt.Printf("[INFO] Shinsakuto Broker starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 4. Start database worker pool
	startWorkers(dataQueue)

	// 5. Setup API Route for Schedulers to push data
	http.HandleFunc("/v1/broker/data", handleIngestion)
	// 6. Setup API Route for Broker status
	http.HandleFunc("/v1/status", statusHandler)

	// Build the listen address string using the new config fields
	listenAddr := fmt.Sprintf("%s:%d", appConfig.Address, appConfig.Port)
	logger.Always("Shinsakuto Broker listening on %s", listenAddr)

	// Start the API Server
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		logger.Fatal("Broker API server crashed: %v", err)
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

	select {
	case dataQueue <- res:
		// Data accepted into the queue
		w.WriteHeader(http.StatusAccepted)
	default:
		// Queue is full, drop data to prevent memory exhaustion
		logger.Info("Broker queue full! Dropping data for %s", res.ID)
		http.Error(w, "Broker overloaded", http.StatusServiceUnavailable)
	}
}

// statusHandler returns current health and port info
func statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "running",
		"address": appConfig.Address,
		"port":    appConfig.Port,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
