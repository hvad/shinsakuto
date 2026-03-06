package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"shinsakuto/pkg/models"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to poller configuration")
	daemonMode := flag.Bool("d", false, "Run poller in background (daemon mode)")
	flag.Parse()

	// Handle Linux Daemonization (-d)
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
			fmt.Printf("[ERROR] Poller: Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("[INFO] Poller starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// Load local configuration
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal Error: %v\n", err)
		os.Exit(1)
	}

	log.Printf("[INFO] Poller %s initialized with %d Scheduler targets", appConfig.PollerID, len(appConfig.SchedulerURLs))

	// Concurrency control using a buffered channel as a semaphore
	sem := make(chan struct{}, appConfig.MaxConcurrent)

	for {
		// Client-side Load Balancing: Iterate through all available Scheduler IPs
		for _, schedulerURL := range appConfig.SchedulerURLs {
			task, err := pullTaskFromURL(schedulerURL)
			if err != nil {
				// Skip this scheduler if it's down or has no tasks for its shard
				continue 
			}

			// Task successfully retrieved
			sem <- struct{}{}
			go func(t models.CheckTask, originURL string) {
				defer func() { <-sem }()
				
				// Execute the check command
				result := executeTask(t)
				
				// Push the result back to the specific Scheduler that issued the task
				pushResultToURL(result, originURL)
			}(task, schedulerURL)
		}

		// Wait for the configured interval before the next polling rotation
		time.Sleep(time.Duration(appConfig.Interval) * time.Millisecond)
	}
}

// pullTaskFromURL attempts to fetch a single task from a specific Scheduler endpoint
func pullTaskFromURL(baseURL string) (models.CheckTask, error) {
	url := fmt.Sprintf("%s/v1/pop-task", baseURL)
	resp, err := httpClient.Get(url)
	if err != nil {
		return models.CheckTask{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.CheckTask{}, fmt.Errorf("no tasks available (Status: %d)", resp.StatusCode)
	}

	var task models.CheckTask
	err = json.NewDecoder(resp.Body).Decode(&task)
	return task, err
}

// pushResultToURL sends the outcome of a check to the originating Scheduler
func pushResultToURL(res models.CheckResult, baseURL string) {
	url := fmt.Sprintf("%s/v1/push-result", baseURL)
	payload, _ := json.Marshal(res)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
	if err == nil {
		defer resp.Body.Close()
		logDebug("Successfully pushed result for %s to %s", res.ID, baseURL)
	} else {
		logDebug("Failed to push result for %s to %s: %v", res.ID, baseURL, err)
	}
}
