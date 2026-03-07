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

	"shinsakuto/pkg/models"
)

var (
	hosts        = make(map[string]*models.Host)
	services     = make(map[string]*models.Service)
	mu           sync.RWMutex
	appConfig    SchedConfig
	statusLogger *log.Logger
	stateChanged bool
	httpClient   = &http.Client{Timeout: 5 * time.Second}
	brokerWG     sync.WaitGroup 
)

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	// Handle Linux Daemonization via process re-execution
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
			fmt.Printf("[ERROR] Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[INFO] Scheduler starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 1. Load configuration and setup logging
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal: Could not load configuration: %v\n", err)
		os.Exit(1)
	}

	initLoggers()
	loadState() //

	// 2. Periodic state persistence (every 1 minute)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState() //
				stateChanged = false
			}
		}
	}()

	// 3. Register HTTP API routes
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)
	mux.HandleFunc("/v1/pop-task", popTaskHandler)
	mux.HandleFunc("/v1/push-result", pushResultHandler)
	mux.HandleFunc("/v1/status", statusHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort),
		Handler: mux,
	}

	// 4. Graceful Shutdown Management
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Scheduler active on port %d (BrokerEnabled: %v)", appConfig.APIPort, appConfig.BrokerEnabled)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Error: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down gracefully...")
	
	// Gracefully stop the HTTP server first
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	}

	// Wait for any pending asynchronous broker requests to finish
	if appConfig.BrokerEnabled {
		log.Println("[INFO] Waiting for pending broker requests...")
		brokerWG.Wait()
	}

	saveState() // Save final state to disk
	log.Println("[INFO] Scheduler stopped safely.")
}
