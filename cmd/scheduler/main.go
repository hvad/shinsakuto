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
)

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	// Handle Daemonization logic
	if *daemonMode {
		args := os.Args[1:]
		// Filter out the -d flag to prevent infinite recursion in the child process
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

	// 1. Initial configuration and logging setup
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal Error: Could not load configuration: %v\n", err)
		os.Exit(1)
	}

	initLoggers()
	loadState() //

	// 2. Periodic background save (every 1 minute)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState() //
				stateChanged = false
			}
		}
	}()

	// 3. Define HTTP API routes
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)   //
	mux.HandleFunc("/v1/pop-task", popTaskHandler)   //
	mux.HandleFunc("/v1/push-result", pushResultHandler) //
	mux.HandleFunc("/v1/status", statusHandler)     //

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort),
		Handler: mux,
	}

	// 4. Signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Shinsakuto Scheduler active on port %d (Debug: %v)", appConfig.APIPort, appConfig.Debug)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Error: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down...")
	
	// Final save before exiting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	}
	
	saveState() //
	log.Println("[INFO] Scheduler stopped safely.")
}
