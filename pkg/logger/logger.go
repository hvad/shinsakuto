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

// Setup initializes the global logging state
func Setup(filePath string, debug bool) {
	debugMode = debug
	logFilePath = filePath

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(f)
		} else {
			log.SetOutput(os.Stdout)
			log.Printf("[ERROR] Could not open log file %s: %v", filePath, err)
		}
	} else {
		log.SetOutput(os.Stdout)
	}
}

// Info writes to the log destination only if debugMode is enabled
func Info(format string, v ...interface{}) {
	// Only trace actions if debug mode is active
	if !debugMode {
		return
	}

	msg := fmt.Sprintf(format, v...)
	
	// Log to the configured output (file or stdout via global log)
	log.Print(msg)

	// If logging to a file, also mirror to stdout for real-time monitoring
	if logFilePath != "" {
		fmt.Fprintf(os.Stdout, "%s [DEBUG] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Fatal writes to logs and terminal, then exits. It ignores the debug flag because it's a critical failure.
func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print("[FATAL] " + msg)
	fmt.Fprintf(os.Stderr, "%s [FATAL] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	os.Exit(1)
}
