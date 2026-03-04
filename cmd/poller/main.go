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
	// Define command-line flags
	configPath := flag.String("config", "config.json", "Path to poller config")
	daemonMode := flag.Bool("d", false, "Run as a daemon in the background")
	flag.Parse()

	// Handle Daemonization: Re-execute the binary in the background
	if *daemonMode {
		args := os.Args[1:]
		// Filter out the -d flag to prevent recursive spawning
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
		
		fmt.Printf("[INFO] Poller starting in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// Load configuration and initialize dual-logging
	if err := loadConfig(*configPath); err != nil {
		fmt.Printf("Fatal: Configuration error: %v\n", err)
		os.Exit(1)
	}

	log.Printf("[INFO] Poller %s started. Connecting to %s", appConfig.PollerID, appConfig.SchedulerURL)

	// Semaphore to strictly limit concurrent check executions
	sem := make(chan struct{}, appConfig.MaxConcurrent)

	for {
		// 1. Pull a task from the Scheduler
		task, err := pullTask()
		if err != nil {
			logDebug("Queue empty or connection issue: %v", err)
			// Wait for the configured interval before retrying
			time.Sleep(time.Duration(appConfig.Interval) * time.Millisecond)
			continue
		}

		// 2. Execute the task using the worker pool
		sem <- struct{}{} // Acquire concurrency slot
		go func(t models.CheckTask) {
			defer func() { <-sem }() // Release slot when finished
			result := executeTask(t)
			pushResult(result)
		}(task)

		// Short sleep to prevent high CPU usage in the polling loop
		time.Sleep(10 * time.Millisecond)
	}
}

// pullTask fetches a pending check from the Scheduler
func pullTask() (models.CheckTask, error) {
	url := fmt.Sprintf("%s/v1/pop-task", appConfig.SchedulerURL)
	resp, err := httpClient.Get(url)
	if err != nil {
		return models.CheckTask{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return models.CheckTask{}, fmt.Errorf("no tasks pending")
	}

	var task models.CheckTask
	err = json.NewDecoder(resp.Body).Decode(&task)
	return task, err
}

// pushResult returns the execution result to the Scheduler
func pushResult(res models.CheckResult) {
	url := fmt.Sprintf("%s/v1/push-result", appConfig.SchedulerURL)
	data, _ := json.Marshal(res)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err == nil {
		defer resp.Body.Close()
		logDebug("Successfully pushed result for %s", res.ID)
	} else {
		logDebug("Failed to push result for %s: %v", res.ID, err)
	}
}
