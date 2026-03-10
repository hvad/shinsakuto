package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// NotificationRequest defines the payload sent to the Reactionner engine.
type NotificationRequest struct {
	EntityID  string    `json:"entity_id"`
	Type      string    `json:"type"`
	State     int       `json:"state"`
	Output    string    `json:"output"`
	Timestamp time.Time `json:"timestamp"`
}

// handleHostResult updates host state and triggers notifications on state change.
func handleHostResult(res models.CheckResult) {
	hID := strings.TrimPrefix(res.ID, "HOST:")
	h, ok := hosts[hID]
	if !ok {
		return
	}

	wasUp := h.IsUp
	h.IsUp = (res.Status == 0)
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
	forwardToBroker(res)
}

// handleServiceResult updates service state and generates alerts if the state changed.
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
		// Suppress notifications if the service or its parent host is in downtime.
		if !s.InDowntime && (!hostExists || !host.InDowntime) {
			notifyReactionner(s.ID, "ALERT", res.Status, res.Output)
		}
	}
	forwardToBroker(res)
}

// forwardToBroker pushes check results to external data brokers (TSDB, Dashboards, etc.).
func forwardToBroker(res models.CheckResult) {
	if !appConfig.BrokerEnabled || len(appConfig.BrokerURLs) == 0 {
		return
	}

	brokerWG.Add(1)
	go func() {
		defer brokerWG.Done()
		payload, _ := json.Marshal(res)
		for _, baseURL := range appConfig.BrokerURLs {
			url := strings.TrimSuffix(baseURL, "/") + "/v1/broker/data"
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(payload))
			if err == nil {
				resp.Body.Close()
				return
			}
		}
	}()
}

// notifyReactionner contacts the notification engine to inform users of issues.
func notifyReactionner(id, t string, state int, out string) {
	logger.Info("Triggering notification for %s", id)
	
	req := NotificationRequest{
		EntityID:  id,
		Type:      t,
		State:     state,
		Output:    out,
		Timestamp: time.Now(),
	}
	
	payload, _ := json.Marshal(req)
	
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		hReq, _ := http.NewRequestWithContext(ctx, "POST", appConfig.ReactionnerURL, bytes.NewBuffer(payload))
		hReq.Header.Set("Content-Type", "application/json")
		
		resp, err := httpClient.Do(hReq)
		if err == nil {
			resp.Body.Close()
		}
	}()
}
