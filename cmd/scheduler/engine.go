package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"shinsakuto/pkg/models"
	"time"
)

// handleHostResult updates host status and logs transitions
func handleHostResult(res models.CheckResult) {
	h, ok := hosts[res.ID]
	if !ok {
		logDebug("Unknown host result received: %s", res.ID)
		return
	}

	wasUp := h.IsUp
	h.IsUp = (res.Status == 0) // Status 0 is considered UP
	h.Status = res.Status
	h.Output = res.Output

	if wasUp != h.IsUp {
		stateStr := "UP"
		if !h.IsUp { stateStr = "DOWN" }
		logDebug("STATE CHANGE: Host %s is now %s", h.ID, stateStr)
		logStateChange("HOST", h.ID, stateStr, res.Output)
		
		if !h.InDowntime {
			notifyReactionner(h.ID, stateStr, res.Status, res.Output)
		} else {
			logDebug("Notification suppressed: %s is in Downtime", h.ID)
		}
	}
}

// handleServiceResult manages service state transitions and alert logic
func handleServiceResult(res models.CheckResult) {
	s, ok := services[res.ID]
	if !ok { return }

	oldState := s.CurrentState
	s.CurrentState = res.Status
	s.Output = res.Output

	if oldState != s.CurrentState {
		labels := map[int]string{0: "OK", 1: "WARNING", 2: "CRITICAL", 3: "UNKNOWN"}
		logDebug("STATE CHANGE: Service %s is now %s", s.ID, labels[res.Status])
		logStateChange("SERVICE", s.ID, labels[res.Status], res.Output)

		host, hostExists := hosts[s.HostName]
		// Alert only if host is UP and no downtime is active for either entity
		if hostExists && host.IsUp && !s.InDowntime && !host.InDowntime {
			notifyReactionner(s.ID, "ALERT", res.Status, res.Output)
		}
	}
}

// notifyReactionner sends alert payloads asynchronously to the Reactionner
func notifyReactionner(id, t string, state int, out string) {
	logDebug("Triggering notification for %s (%s)", id, t)
	notification := models.NotificationRequest{
		EntityID:  id,
		Type:      t,
		State:     state,
		Output:    out,
		Timestamp: time.Now(),
	}
	
	payload, _ := json.Marshal(notification)
	
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		req, _ := http.NewRequestWithContext(ctx, "POST", appConfig.ReactionnerURL, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := httpClient.Do(req)
		if err != nil {
			logDebug("Failed to contact Reactionner: %v", err)
			return
		}
		logDebug("Reactionner accepted notification for %s (Status: %d)", id, resp.StatusCode)
		resp.Body.Close()
	}()
}
