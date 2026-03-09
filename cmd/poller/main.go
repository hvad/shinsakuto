package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// Shared HTTP client with a defined timeout for network operations
var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	// Parse command-line flags for configuration path and daemon mode
	configPath := flag.String("c", "config.json", "Path to poller configuration")
	daemonMode := flag.Bool("d", false, "Run poller in background")
	flag.Parse()

	// 1. Load the JSON configuration and initialize the logging system
	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Could not load config: %v\n", err)
		os.Exit(1)
	}
	initLogger()

	// 2. Handle Daemonization: re-execute the binary without the -d flag in the background
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
			logger.Fatal("Failed to start daemon: %v", err)
		}
		
		fmt.Printf("[INFO] Poller %s starting in background (PID: %d)\n", appConfig.PollerID, cmd.Process.Pid)
		os.Exit(0)
	}

	// Logging initialization info (only visible if Debug is true in config)
	logger.Info("Poller %s initialized with %d Schedulers", appConfig.PollerID, len(appConfig.SchedulerURLs))

	// 3. Concurrency control using a buffered channel as a semaphore
	sem := make(chan struct{}, appConfig.MaxConcurrent)

	for {
		// Iterate through all configured Schedulers to distribute load
		for _, schedulerURL := range appConfig.SchedulerURLs {
			task, err := pullTaskFromURL(schedulerURL)
			if err != nil {
				// Skip silently if no tasks are available or scheduler is down
				continue 
			}

			// Acquire a slot in the semaphore
			sem <- struct{}{}
			go func(t models.CheckTask, originURL string) {
				defer func() { <-sem }()
				
				// Execute the check and report back to the specific scheduler
				result := executeTask(t)
				pushResultToURL(result, originURL)
			}(task, schedulerURL)
		}

		// Wait for the configured interval before the next polling cycle
		time.Sleep(time.Duration(appConfig.IntervalMS) * time.Millisecond)
	}
}

// pullTaskFromURL fetches a single task from a Scheduler's queue
func pullTaskFromURL(baseURL string) (models.CheckTask, error) {
	url := fmt.Sprintf("%s/v1/pop-task", baseURL)
	resp, err := httpClient.Get(url)
	if err != nil {
		return models.CheckTask{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.CheckTask{}, fmt.Errorf("no tasks available")
	}

	var task models.CheckTask
	err = json.NewDecoder(resp.Body).Decode(&task)
	return task, err
}

// pushResultToURL sends the execution outcome back to the Scheduler
func pushResultToURL(res models.CheckResult, baseURL string) {
	url := fmt.Sprintf("%s/v1/push-result", baseURL)
	payload, _ := json.Marshal(res)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
	if err == nil {
		resp.Body.Close()
		logger.Info("[NETWORK] Successfully pushed result for %s to %s", res.ID, baseURL)
	} else {
		// Log network errors (only if Debug is enabled)
		logger.Info("[ERROR] Failed to push result for %s to %s: %v", res.ID, baseURL, err)
	}
}
