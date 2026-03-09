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

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// Global variables shared across the main package
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
	configPath := flag.String("c", "config.json", "Path to configuration file")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Could not load configuration: %v\n", err)
		os.Exit(1)
	}

	initLoggers()
	loadState()

	if *daemonMode {
		cmd := exec.Command(os.Args[0], "-config", *configPath)
		if err := cmd.Start(); err != nil {
			logger.Fatal("Failed to start daemon: %v", err) //
		}
		fmt.Printf("[INFO] Scheduler starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// Periodic state persistence loop
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState()
				stateChanged = false
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)
	mux.HandleFunc("/v1/pop-task", popTaskHandler)
	mux.HandleFunc("/v1/push-result", pushResultHandler)
	mux.HandleFunc("/v1/status", statusHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort),
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("Scheduler active on port %d (Broker: %v)", appConfig.APIPort, appConfig.BrokerEnabled)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server Error: %v", err)
		}
	}()

	<-stop
	logger.Info("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Info("[ERROR] Server forced to shutdown: %v", err)
	}

	if appConfig.BrokerEnabled {
		logger.Info("Waiting for pending broker requests...")
		brokerWG.Wait()
	}

	saveState()
	logger.Info("Scheduler stopped safely.")
}
