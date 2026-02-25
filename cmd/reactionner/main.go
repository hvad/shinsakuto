package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"sync"
	"time"

	"go-shinken/pkg/models"
)

// --- Configuration Structures ---

type Config struct {
	APIPort   int        `json:"api_port"`
	Debug     bool       `json:"debug"`
	SystemLog string     `json:"system_log"`
	AlertsLog string     `json:"alerts_log"`
	SMTP      SMTPConfig `json:"smtp"`
}

type SMTPConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// --- Global Variables ---

var (
	appConfig       Config
	systemLogger    *log.Logger
	alertLogger     *log.Logger
	maintenances    = make(map[string]time.Time)
	acknowledgments = make(map[string]bool)
	mu              sync.RWMutex
)

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	isDaemon := flag.Bool("d", false, "Run as background daemon")
	debugFlag := flag.Bool("debug", false, "Enable verbose debug mode")
	flag.Parse()

	if err := loadConfig(*configPath); err != nil {
		log.Fatalf("Fatal: Failed to load configuration: %v", err)
	}

	if *debugFlag {
		appConfig.Debug = true
	}

	initLoggers()

	if *isDaemon && os.Getenv("REACTIONNER_DAEMON") != "true" {
		daemonize(*configPath)
		return
	}

	logDebug("[System] Debug mode enabled")
	systemLogger.Printf("[System] Reactionner started on port %d", appConfig.APIPort)

	// API Routes
	http.HandleFunc("/notify", notifyHandler)
	http.HandleFunc("/ack", ackHandler)
	http.HandleFunc("/maintenance", maintenanceHandler)
	http.HandleFunc("/status", statusHandler)

	systemLogger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", appConfig.APIPort), nil))
}

func initLoggers() {
	sysFile, _ := os.OpenFile(appConfig.SystemLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	systemLogger = log.New(sysFile, "SYS: ", log.LstdFlags)

	alFile, _ := os.OpenFile(appConfig.AlertsLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	alertLogger = log.New(alFile, "ALERT: ", log.LstdFlags)
}

func logDebug(msg string, args ...interface{}) {
	if appConfig.Debug {
		fmt.Printf("[DEBUG] "+msg+"\n", args...)
	}
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {
	var req models.NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	logDebug("Notification received for: %s (Type: %s)", req.EntityID, req.Type)

	mu.Lock()
	if req.Type == "RECOVERY" {
		delete(acknowledgments, req.EntityID)
	}

	if until, ok := maintenances[req.EntityID]; ok {
		if time.Now().Before(until) {
			systemLogger.Printf("[Muted] %s is in maintenance", req.EntityID)
			mu.Unlock()
			return
		}
		delete(maintenances, req.EntityID)
	}

	if acknowledgments[req.EntityID] {
		logDebug("Skipping: %s is acknowledged", req.EntityID)
		mu.Unlock()
		return
	}
	mu.Unlock()

	alertLogger.Printf("[%s] %s | State: %d | Output: %s", req.Type, req.EntityID, req.State, req.Output)

	go sendEmail(req)
	w.WriteHeader(http.StatusOK)
}

func sendEmail(req models.NotificationRequest) {
	if !appConfig.SMTP.Enabled {
		return
	}

	addr := fmt.Sprintf("%s:%d", appConfig.SMTP.Host, appConfig.SMTP.Port)
	subject := fmt.Sprintf("Subject: [%s] %s\n", req.Type, req.EntityID)
	body := fmt.Sprintf("To: %s\n%s\nEntity: %s\nState: %d\nOutput: %s", 
		appConfig.SMTP.To, subject, req.EntityID, req.State, req.Output)

	auth := smtp.PlainAuth("", appConfig.SMTP.Username, appConfig.SMTP.Password, appConfig.SMTP.Host)

	if appConfig.SMTP.Port == 465 {
		// Secure Method: Explicit TLS (SSL)
		tlsConfig := &tls.Config{ServerName: appConfig.SMTP.Host}
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			systemLogger.Printf("[Error] TCP Dial failed: %v", err)
			return
		}
		tlsConn := tls.Client(conn, tlsConfig)
		client, err := smtp.NewClient(tlsConn, appConfig.SMTP.Host)
		if err != nil { return }
		defer client.Quit()

		if err = client.Auth(auth); err != nil { return }
		if err = client.Mail(appConfig.SMTP.From); err != nil { return }
		if err = client.Rcpt(appConfig.SMTP.To); err != nil { return }
		w, _ := client.Data()
		w.Write([]byte(body))
		w.Close()
	} else {
		// Standard Method: STARTTLS
		err := smtp.SendMail(addr, auth, appConfig.SMTP.From, []string{appConfig.SMTP.To}, []byte(body))
		if err != nil {
			systemLogger.Printf("[Error] SMTP Fail: %v", err)
		}
	}
}

// --- Standard Handlers & Utilities ---

func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil { return err }
	return json.Unmarshal(data, &appConfig)
}

func daemonize(configPath string) {
	cmd := exec.Command(os.Args[0], "-config", configPath)
	cmd.Env = append(os.Environ(), "REACTIONNER_DAEMON=true")
	cmd.Start()
	os.Exit(0)
}

func ackHandler(w http.ResponseWriter, r *http.Request) {
	var body struct{ EntityID string `json:"entity_id"` }
	json.NewDecoder(r.Body).Decode(&body)
	mu.Lock()
	acknowledgments[body.EntityID] = true
	mu.Unlock()
}

func maintenanceHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EntityID string `json:"entity_id"`
		Minutes  int    `json:"minutes"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	mu.Lock()
	maintenances[body.EntityID] = time.Now().Add(time.Duration(body.Minutes) * time.Minute)
	mu.Unlock()
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{"maintenances": maintenances, "acks": acknowledgments})
}
