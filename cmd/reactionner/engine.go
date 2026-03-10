package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// processNotification evaluates if an alert should be sent or muted
func processNotification(req models.NotificationRequest) {
	// In HA mode, only the leader is allowed to send notifications
	if !isLeader() {
		return
	}

	mu.RLock()
	// 1. Maintenance Check: Mute if entity is in a scheduled downtime
	if until, ok := maintenances[req.EntityID]; ok {
		if time.Now().Unix() < until {
			logger.Info("[ENGINE] Notification muted: %s is in maintenance mode", req.EntityID)
			mu.RUnlock()
			return
		}
	}

	// 2. Acknowledgment Check: Mute if a technician has already claimed the issue
	if acknowledgments[req.EntityID] {
		logger.Info("[ENGINE] Notification skipped: %s is already acknowledged", req.EntityID)
		mu.RUnlock()
		return
	}
	mu.RUnlock()

	// 3. Alert Auditing: Write to the dedicated alert history file (AlertsLog)
	alertLogger.Printf("[%s] %s | State: %d | Output: %s",
		req.Type, req.EntityID, req.State, req.Output)

	// 4. Trigger Reactions (e.g., Email)
	go sendEmail(req)
}

// sendEmail formats and dispatches the alert via SMTP
func sendEmail(req models.NotificationRequest) {
	if !appConfig.SMTP.Enabled {
		return
	}

	addr := fmt.Sprintf("%s:%d", appConfig.SMTP.Host, appConfig.SMTP.Port)
	subject := fmt.Sprintf("Subject: [%s] %s\n", req.Type, req.EntityID)

	// Message Construction
	body := fmt.Sprintf("To: %s\n%s\n\n--- Shinsakuto Alert ---\nEntity: %s\nType: %s\nState: %d\nOutput: %s\nTime: %s",
		appConfig.SMTP.To, subject, req.EntityID, req.Type, req.State, req.Output, time.Now().Format(time.RFC822))

	auth := smtp.PlainAuth("", appConfig.SMTP.Username, appConfig.SMTP.Password, appConfig.SMTP.Host)

	logger.Info("[SMTP] Sending alert for %s to %s", req.EntityID, appConfig.SMTP.To)

	// Support for SMTPS (Port 465) or standard SMTP/STARTTLS
	if appConfig.SMTP.Port == 465 {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         appConfig.SMTP.Host,
		}

		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			logger.Info("[ERROR] SMTP Connection failed: %v", err)
			return
		}

		client, err := smtp.NewClient(tls.Client(conn, tlsConfig), appConfig.SMTP.Host)
		if err != nil {
			logger.Info("[ERROR] SMTP TLS handshake failed: %v", err)
			return
		}
		defer client.Quit()

		if err = client.Auth(auth); err != nil {
			logger.Info("[ERROR] SMTP Auth failed: %v", err)
			return
		}

		client.Mail(appConfig.SMTP.From)
		client.Rcpt(appConfig.SMTP.To)

		w, _ := client.Data()
		w.Write([]byte(body))
		w.Close()
	} else {
		// Standard SMTP delivery
		err := smtp.SendMail(addr, auth, appConfig.SMTP.From, []string{appConfig.SMTP.To}, []byte(body))
		if err != nil {
			logger.Info("[ERROR] SMTP SendMail failed: %v", err)
		}
	}
}
