package main

import (
	"bytes"
	"fmt"
	"net/http"
	"shinsakuto/pkg/models" //
	"time"
)

// startWorkers initializes the pool of background database writers
func startWorkers(dataChan <-chan models.CheckResult) {
	for i := 0; i < appConfig.WorkerCount; i++ {
		go dbWorker(dataChan, i)
	}
}

// dbWorker processes the queue and sends batches to the TSDB
func dbWorker(dataChan <-chan models.CheckResult, id int) {
	batch := make([]models.CheckResult, 0, appConfig.BatchSize)
	ticker := time.NewTicker(time.Duration(appConfig.FlushIntervalMS) * time.Millisecond)
	defer ticker.Stop()

	logDebug("DB Worker %d started", id)

	for {
		select {
		case res := <-dataChan:
			batch = append(batch, res)
			if len(batch) >= appConfig.BatchSize {
				flushToTSDB(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				logDebug("Worker %d: Interval flush triggering", id)
				flushToTSDB(batch)
				batch = batch[:0]
			}
		}
	}
}

// flushToTSDB converts results to Line Protocol and sends the POST request
func flushToTSDB(results []models.CheckResult) {
	var buffer bytes.Buffer
	now := time.Now().UnixNano()

	for _, res := range results {
		// Influx Line Protocol format: measurement,tag=val field=val timestamp
		line := fmt.Sprintf("shinsakuto_check,id=%s status=%di,output=\"%s\" %d\n",
			res.ID, res.Status, res.Output, now)
		buffer.WriteString(line)
	}

	req, err := http.NewRequest("POST", appConfig.TSDBUrl, &buffer)
	if err != nil {
		log.Printf("[ERROR] Failed to create TSDB request: %v", err)
		return
	}

	if appConfig.TSDBToken != "" {
		req.Header.Set("Authorization", "Token "+appConfig.TSDBToken)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[ERROR] TSDB network error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("[ERROR] TSDB returned status: %d", resp.StatusCode)
	}
}
