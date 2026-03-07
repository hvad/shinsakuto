package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"shinsakuto/pkg/models"
	"strings"
	"time"
)

// handleHostResult processes host check results and handles state transitions
func handleHostResult(res models.CheckResult) {
	hID := strings.TrimPrefix(res.ID, "HOST:")
	h, ok := hosts[hID]
	if !ok {
		return
	}

	wasUp := h.IsUp
	h.IsUp = (res.Status == 0) // 0 is OK/UP
	h.Status, h.Output = res.Status, res.Output

	if wasUp != h.IsUp {
		state := "UP"
		if !h.IsUp {
			state = "DOWN"
		}
		logStateChange("HOST", h.ID, state, res.Output)
		if !h.InDowntime {
			notifyReactionner(h.ID, state, res.Status, res.Output)
		}
	}

	// Forward result to broker for storage if enabled
	forwardToBroker(res)
}

// handleServiceResult processes service check results and state changes
func handleServiceResult(res models.CheckResult) {
	s, ok := services[res.ID]
	if !ok {
		return
	}

	oldState := s.CurrentState
	s.CurrentState, s.Output = res.Status, res.Output

	if oldState != s.CurrentState {
		logStateChange("SERVICE", s.ID, "CHANGE", res.Output)
		host, hostExists := hosts[s.HostName]
		if !s.InDowntime && (!hostExists || !host.InDowntime) {
			notifyReactionner(s.ID, "ALERT", res.Status, res.Output)
		}
	}

	// Forward result to broker for storage if enabled
	forwardToBroker(res)
}

// forwardToBroker sends check results to the first available Broker (Client-side Failover)
func forwardToBroker(res models.CheckResult) {
	if !appConfig.BrokerEnabled || len(appConfig.BrokerURLs) == 0 {
		return
	}

	brokerWG.Add(1) // Track this async task for graceful shutdown
	go func() {
		defer brokerWG.Done()
		payload, _ := json.Marshal(res)

		// Try each configured broker URL until one succeeds
		for _, baseURL := range appConfig.BrokerURLs {
			url := strings.TrimSuffix(baseURL, "/") + "/v1/broker/data"
			
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return // Success, data delivered
				}
				logDebug("[BROKER] %s returned status %d", url, resp.StatusCode)
			} else {
				logDebug("[BROKER] Failed to reach %s: %v", url, err)
			}
		}
		logDebug("[ERROR] All brokers failed to receive result for %s", res.ID)
	}()
}

// notifyReactionner sends alert payloads asynchronously to the Reactionner
func notifyReactionner(id, t string, state int, out string) {
	logDebug("Triggering notification for %s", id)
	payload, _ := json.Marshal(models.NotificationRequest{
		EntityID: id, Type: t, State: state, Output: out, Timestamp: time.Now(),
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "POST", appConfig.ReactionnerURL, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}
