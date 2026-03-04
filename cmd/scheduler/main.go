package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	startTime = time.Now()
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	// 1. Load configuration parameters
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize loggers and restore previous state
	initLoggers()
	loadState()

	// 3. Start periodic state persistence routine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if stateChanged {
				saveState()
				stateChanged = false
			}
		}
	}()

	// 4. Setup API Router
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sync-all", syncAllHandler)
	mux.HandleFunc("/v1/push-result", pushResultHandler)
	mux.HandleFunc("/v1/status", statusHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort),
		Handler: mux,
	}

	// 5. Signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Shinsakuto Scheduler started on %s:%d (Debug: %v)", 
			appConfig.APIAddress, appConfig.APIPort, appConfig.Debug)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down Scheduler...")
	
	// Shutdown the server gracefully with a 5-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	server.Shutdown(ctx)
	saveState() // Ensure the final state is captured before exit
	log.Println("[INFO] Shutdown complete.")
}
