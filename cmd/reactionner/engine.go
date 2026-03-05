package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"shinsakuto/pkg/models" //
	"time"
)

// processNotification handles alert logic and deduplication
func processNotification(req models.NotificationRequest) {
	// Only the leader processes and sends notifications in HA mode
	if !isLeader() {
		return
	}

	mu.RLock()
	// Check maintenance
	if until, ok := maintenances[req.EntityID]; ok {
		if time.Now().Unix() < until {
			logDebug("Notification muted: %s is in maintenance", req.EntityID)
			mu.RUnlock()
			return
		}
	}
	// Check Acknowledgment
	if acknowledgments[req.EntityID] {
		logDebug("Notification skipped: %s is already acknowledged", req.EntityID)
		mu.RUnlock()
		return
	}
	mu.RUnlock()

	// Normal logging for alerts
	alertLogger.Printf("[%s] %s | State: %d | Output: %s", req.Type, req.EntityID, req.State, req.Output)
	
	go sendEmail(req)
}

func sendEmail(req models.NotificationRequest) {
	if !appConfig.SMTP.Enabled { return }
	
	addr := fmt.Sprintf("%s:%d", appConfig.SMTP.Host, appConfig.SMTP.Port)
	subject := fmt.Sprintf("Subject: [%s] %s\n", req.Type, req.EntityID)
	body := fmt.Sprintf("To: %s\n%s\nEntity: %s\nState: %d\nOutput: %s", 
		appConfig.SMTP.To, subject, req.EntityID, req.State, req.Output)

	auth := smtp.PlainAuth("", appConfig.SMTP.Username, appConfig.SMTP.Password, appConfig.SMTP.Host)

	logDebug("Attempting to send email alert for %s", req.EntityID)

	if appConfig.SMTP.Port == 465 {
		tlsConfig := &tls.Config{ServerName: appConfig.SMTP.Host}
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			systemLogger.Printf("[ERROR] SMTP Connection failed: %v", err)
			return
		}
		client, _ := smtp.NewClient(tls.Client(conn, tlsConfig), appConfig.SMTP.Host)
		defer client.Quit()
		client.Auth(auth)
		client.Mail(appConfig.SMTP.From)
		client.Rcpt(appConfig.SMTP.To)
		w, _ := client.Data()
		w.Write([]byte(body))
		w.Close()
	} else {
		err := smtp.SendMail(addr, auth, appConfig.SMTP.From, []string{appConfig.SMTP.To}, []byte(body))
		if err != nil {
			systemLogger.Printf("[ERROR] SMTP SendMail failed: %v", err)
		}
	}
}
