package main

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"shinsakuto/pkg/models"
	"shinsakuto/pkg/logger"
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

	logger.Info("DB Worker %d initialized and waiting for data", id)

	for {
		select {
		case res := <-dataChan:
			batch = append(batch, res)
			// Flush if batch size is reached
			if len(batch) >= appConfig.BatchSize {
				flushToTSDB(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			// Periodic flush to avoid data being stuck in the buffer
			if len(batch) > 0 {
				logger.Info("Worker %d: Interval flush triggered (batch size: %d)", id, len(batch))
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
		// Format: measurement,tag=val field=val timestamp (Influx Line Protocol)
		line := fmt.Sprintf("shinsakuto_check,id=%s status=%di,output=\"%s\" %d\n",
			res.ID, res.Status, res.Output, now)
		buffer.WriteString(line)
	}

	req, err := http.NewRequest("POST", appConfig.TSDBUrl, &buffer)
	if err != nil {
		logger.Info("[ERROR] Failed to create TSDB request: %v", err)
		return
	}

	// Add authentication header if token is provided
	if appConfig.TSDBToken != "" {
		req.Header.Set("Authorization", "Token "+appConfig.TSDBToken)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Info("[ERROR] TSDB network error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		logger.Info("[ERROR] TSDB returned unexpected status: %d", resp.StatusCode)
	}
}
