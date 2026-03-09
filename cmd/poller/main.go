package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// Shared HTTP client with a 10s timeout to prevent hanging connections
var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	// Parse command-line flags
	configPath := flag.String("c", "config.json", "Path to poller configuration")
	daemonMode := flag.Bool("d", false, "Run poller in background")
	flag.Parse()

	// 1. Initialize configuration and logger
	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Could not load config: %v\n", err)
		os.Exit(1)
	}
	initLogger()

	// 2. Handle background execution (daemon mode)
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

	// 3. Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Log lifecycle start event (always logged regardless of debug level)
	logger.Always("Poller %s version 1.0 starting... [Interval: %dms]", appConfig.PollerID, appConfig.IntervalMS)

	// 4. Concurrency control via semaphore (channel)
	sem := make(chan struct{}, appConfig.MaxConcurrent)

	// Start the main polling loop in a goroutine
	go func() {
		for {
			for _, schedulerURL := range appConfig.SchedulerURLs {
				task, err := pullTaskFromURL(schedulerURL)
				if err != nil {
					// Silent skip if no tasks or scheduler is unreachable
					continue 
				}

				// Acquire semaphore slot
				sem <- struct{}{}
				go func(t models.CheckTask, originURL string) {
					defer func() { <-sem }()
					
					// Execute the command and report result back to the specific scheduler
					result := executeTask(t)
					pushResultToURL(result, originURL)
				}(task, schedulerURL)
			}

			// Respect the configured polling interval
			time.Sleep(time.Duration(appConfig.IntervalMS) * time.Millisecond)
		}
	}()

	// Block until a signal is received
	sig := <-stop
	logger.Always("Poller %s received signal (%v). Shutting down gracefully.", appConfig.PollerID, sig)
	os.Exit(0)
}

// pullTaskFromURL fetches a task from a Scheduler's pop-task endpoint
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

// pushResultToURL sends the command execution outcome back to the Scheduler
func pushResultToURL(res models.CheckResult, baseURL string) {
	url := fmt.Sprintf("%s/v1/push-result", baseURL)
	payload, _ := json.Marshal(res)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
	if err == nil {
		resp.Body.Close()
		// Debug level log for successful network operation
		logger.Info("[NETWORK] Successfully pushed result for task %s to %s", res.ID, baseURL)
	} else {
		// Log failures even in non-debug mode to identify network issues
		logger.Info("[ERROR] Failed to push result for task %s to %s: %v", res.ID, baseURL, err)
	}
}
