package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"shinsakuto/pkg/models" // Using the provided models
)

var (
	hosts        = make(map[string]*models.Host)
	services     = make(map[string]*models.Service)
	mu           sync.RWMutex
	appConfig    SchedConfig
	statusLogger *log.Logger
	stateChanged bool
	httpClient   = &http.Client{Timeout: 5 * time.Second}
)

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	// Handle Linux Daemonization
	if *daemonMode {
		args := os.Args[1:]
		newArgs := make([]string, 0)
		for _, arg := range args {
			// Ensure the child process doesn't try to daemonize again
			if arg != "-d" {
				newArgs = append(newArgs, arg)
			}
		}

		cmd := exec.Command(os.Args[0], newArgs...)
		if err := cmd.Start(); err != nil {
			fmt.Printf("[ERROR] Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("[INFO] Scheduler starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 1. Configuration and logging setup
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal: Could not load configuration: %v\n", err)
		os.Exit(1)
	}

	initLoggers()
	loadState() // Restore previous state from disk

	// 2. Periodic background save cycle
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState() // Persistent storage of monitored object states
				stateChanged = false
			}
		}
	}()

	// 3. API Route Registration
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)   // Arbiter sync endpoint
	mux.HandleFunc("/v1/pop-task", popTaskHandler)   // Poller task request
	mux.HandleFunc("/v1/push-result", pushResultHandler) // Poller result submission
	mux.HandleFunc("/v1/status", statusHandler)     // Health and status endpoint

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort),
		Handler: mux,
	}

	// 4. Graceful Shutdown Management
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Scheduler active on port %d (Daemon: %v)", appConfig.APIPort, *daemonMode)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Error: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down gracefully...")
	
	// Finalize server operations with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Shutdown failed: %v", err)
	}

	saveState() // Final state persistence before exit
	log.Println("[INFO] Scheduler stopped safely.")
}
