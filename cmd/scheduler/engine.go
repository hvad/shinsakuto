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
		} else {
			logDebug("Host %s alert suppressed: Active Downtime", h.ID)
		}
	}
}

// handleServiceResult processes service check results
func handleServiceResult(res models.CheckResult) {
	s, ok := services[res.ID]
	if !ok {
		return
	}

	oldState := s.CurrentState
	s.CurrentState, s.Output = res.Status, res.Output

	if oldState != s.CurrentState {
		logStateChange("SERVICE", s.ID, "CHANGE", res.Output)
		// Only alert if the service and its host are not in downtime
		host, hostExists := hosts[s.HostName]
		if !s.InDowntime && (!hostExists || !host.InDowntime) {
			notifyReactionner(s.ID, "ALERT", res.Status, res.Output)
		}
	}
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

// forwardToBroker send result to broker asynchronously
func forwardToBroker(res models.CheckResult) {
	if !appConfig.BrokerEnabled || appConfig.BrokerURL == "" {
		return
	}

	go func() {
		payload, _ := json.Marshal(res)
		resp, err := httpClient.Post(appConfig.BrokerURL+"/v1/broker/data", "application/json", bytes.NewBuffer(payload))
		if err != nil {
			logDebug("[BROKER] Erreur d'envoi: %v", err)
			return
		}
		resp.Body.Close()
	}()
}
