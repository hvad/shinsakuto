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
	"strings"
	"sync"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// Global variables shared across the scheduler
var (
	hosts        = make(map[string]*models.Host)
	services     = make(map[string]*models.Service)
	mu           sync.RWMutex
	appConfig    SchedConfig
	statusLogger *log.Logger
	stateChanged bool
	httpClient   = &http.Client{Timeout: 5 * time.Second}
	brokerWG     sync.WaitGroup
	// resultQueue decouples HTTP reception from logic processing to prevent saturation
	resultQueue = make(chan models.CheckResult, 5000)
)

func main() {
	configPath := flag.String("c", "config.json", "Path to configuration file")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	// 1. Load configuration
	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Could not load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize loggers and restore state from disk
	initLoggers()
	loadState()

	// 3. Handle Daemonization
	if *daemonMode {
		cmd := exec.Command(os.Args[0], "-c", *configPath)
		if err := cmd.Start(); err != nil {
			logger.Fatal("Failed to start daemon: %v", err)
		}
		fmt.Printf("[INFO] Scheduler starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 4. Start asynchronous workers to process incoming check results
	for i := 0; i < 10; i++ {
		go resultWorker()
	}

	// 5. Periodic state persistence loop
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState()
				stateChanged = false
			}
		}
	}()

	// 6. Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)
	mux.HandleFunc("/v1/pop-task", popTaskHandler)
	mux.HandleFunc("/v1/push-result", pushResultHandler)
	mux.HandleFunc("/v1/status", statusHandler)

	listenAddr := fmt.Sprintf("%s:%d", appConfig.Address, appConfig.Port)
	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	// 7. Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Always("Scheduler listening on %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server Error: %v", err)
		}
	}()

	<-stop
	logger.Always("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Info("[ERROR] Server forced to shutdown: %v", err)
	}

	// Wait for any pending broker updates
	if appConfig.BrokerEnabled {
		logger.Info("Waiting for pending broker requests...")
		brokerWG.Wait()
	}

	saveState() // Final persistence before exit
	logger.Always("Scheduler stopped safely.")
}

// resultWorker consumes results from the channel and applies updates under a lock
func resultWorker() {
	for res := range resultQueue {
		mu.Lock()
		if strings.HasPrefix(res.ID, "HOST:") {
			handleHostResult(res)
		} else {
			handleServiceResult(res)
		}
		stateChanged = true
		mu.Unlock()
	}
}
