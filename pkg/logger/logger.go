package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	debugMode   bool
	logFilePath string
)

// Setup initializes the global logging state.
// It sets whether debug traces are enabled and redirects output to a file if provided.
func Setup(filePath string, debug bool) {
	debugMode = debug
	logFilePath = filePath

	if filePath != "" {
		// Open the log file in append mode, create it if it doesn't exist
		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			// Redirect global Go log to the file
			log.SetOutput(f)
		} else {
			// Fallback to Stdout if the file cannot be opened
			log.SetOutput(os.Stdout)
			log.Printf("[ERROR] Could not open log file %s: %v", filePath, err)
		}
	} else {
		// Default output is Stdout if no file path is specified
		log.SetOutput(os.Stdout)
	}
}

// Always writes to the log file and terminal regardless of the debug mode.
// This is used for critical lifecycle events like service start and stop.
func Always(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	
	// Log to the internal logger (file or default output)
	log.Print("[INFO] " + msg)

	// Always print lifecycle events to the console for visibility (e.g., systemd/docker logs)
	fmt.Fprintf(os.Stdout, "%s [INFO] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
}

// Info writes to the log file and terminal ONLY if debugMode is enabled.
// This is used for routine tracing such as task execution and network requests.
func Info(format string, v ...interface{}) {
	// If debug is disabled in the config, we do nothing to save I/O
	if !debugMode {
		return
	}

	msg := fmt.Sprintf(format, v...)
	
	// Write to the internal logger
	log.Print("[DEBUG] " + msg)

	// If a file is being used, mirror the debug output to terminal for real-time monitoring
	if logFilePath != "" {
		fmt.Fprintf(os.Stdout, "%s [DEBUG] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Fatal writes a fatal error message to logs and terminal, then exits the program.
func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print("[FATAL] " + msg)
	
	// Write to Stderr to ensure the error is seen during a crash
	fmt.Fprintf(os.Stderr, "%s [FATAL] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	os.Exit(1)
}
