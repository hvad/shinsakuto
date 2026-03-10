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

var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	configPath := flag.String("c", "config.json", "Path to poller configuration")
	daemonMode := flag.Bool("d", false, "Run poller in background")
	flag.Parse()

	if err := loadConfig(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Could not load config: %v\n", err)
		os.Exit(1)
	}
	initLogger()

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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	logger.Always("Poller %s starting... [Interval: %dms]", appConfig.PollerID, appConfig.IntervalMS)

	sem := make(chan struct{}, appConfig.MaxConcurrent)

	go func() {
		for {
			for _, schedulerURL := range appConfig.SchedulerURLs {
				// On récupère les données brutes car le Scheduler peut envoyer un Host ou un Service
				taskData, err := pullTaskFromURL(schedulerURL)
				if err != nil {
					continue 
				}

				sem <- struct{}{}
				go func(data []byte, originURL string) {
					defer func() { <-sem }()
					
					// Extraction de la commande et de l'ID (compatible Host et Service)
					result := executeTask(data)
					pushResultToURL(result, originURL)
				}(taskData, schedulerURL)
			}
			time.Sleep(time.Duration(appConfig.IntervalMS) * time.Millisecond)
		}
	}()

	sig := <-stop
	logger.Always("Poller %s received signal (%v). Shutting down gracefully.", appConfig.PollerID, sig)
	os.Exit(0)
}

// pullTaskFromURL retourne maintenant []byte pour permettre un décodage flexible dans l'executor
func pullTaskFromURL(baseURL string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/pop-task", baseURL)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("no tasks available")
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

func pushResultToURL(res models.CheckResult, baseURL string) {
	url := fmt.Sprintf("%s/v1/push-result", baseURL)
	payload, _ := json.Marshal(res)
	
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
	if err == nil {
		resp.Body.Close()
		logger.Info("[NETWORK] Successfully pushed result for task %s to %s", res.ID, baseURL)
	} else {
		logger.Info("[ERROR] Failed to push result for task %s to %s: %v", res.ID, baseURL, err)
	}
}
