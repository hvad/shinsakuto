package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"shinsakuto/pkg/models"
	"strings"
	"time"

	"shinsakuto/pkg/logger"
)

func handleHostResult(res models.CheckResult) {
	hID := strings.TrimPrefix(res.ID, "HOST:")
	mu.Lock()
	h, ok := hosts[hID]
	mu.Unlock()
	if !ok { return }

	wasUp := h.IsUp
	h.IsUp = (res.Status == 0)
	h.Status, h.Output = res.Status, res.Output

	if wasUp != h.IsUp {
		state := "UP"
		if !h.IsUp { state = "DOWN" }
		logStateChange("HOST", h.ID, state, res.Output)
		if !h.InDowntime {
			notifyReactionner(h.ID, state, res.Status, res.Output)
		}
	}
	forwardToBroker(res)
}

func handleServiceResult(res models.CheckResult) {
	mu.Lock()
	s, ok := services[res.ID] 
	mu.Unlock()
	if !ok { return }

	oldState := s.CurrentState
	s.CurrentState, s.Output = res.Status, res.Output

	if oldState != s.CurrentState {
		logStateChange("SERVICE", s.ID, "CHANGE", res.Output)
		mu.RLock()
		host, hostExists := hosts[s.HostName]
		mu.RUnlock()
		if !s.InDowntime && (!hostExists || !host.InDowntime) {
			notifyReactionner(s.ID, "ALERT", res.Status, res.Output)
		}
	}
	forwardToBroker(res)
}

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
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return
				}
				logger.Info("[BROKER] %s returned status %d", url, resp.StatusCode)
			} else {
				logger.Info("[BROKER] Failed to reach %s: %v", url, err)
			}
		}
		logger.Info("[ERROR] All brokers failed to receive result for %s", res.ID)
	}()
}

func notifyReactionner(id, t string, state int, out string) {
	logger.Info("Triggering notification for %s", id)
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
		} else {
			logger.Info("[ERROR] Failed to reach Reactionner: %v", err)
		}
	}()
}
