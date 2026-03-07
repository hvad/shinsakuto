package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	debugMode bool
	logFilePath string
)

// Setup initializes the global logging state
func Setup(filePath string, debug bool) {
	debugMode = debug
	logFilePath = filePath

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			// Redirect global Go log to the file
			log.SetOutput(f)
		} else {
			log.SetOutput(os.Stdout)
			log.Printf("[ERROR] Could not open log file %s: %v", filePath, err)
		}
	} else {
		log.SetOutput(os.Stdout)
	}
}

// Info writes to the log file, and to terminal only if debug is enabled
func Info(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	
	// Always log to the internal logger (file)
	log.Print(msg)

	// Log to terminal only if debug is on and a file is being used
	if debugMode && logFilePath != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	}
}

// Fatal writes to logs and terminal, then exits
func Fatal(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print("[FATAL] " + msg)
	fmt.Fprintf(os.Stderr, "%s [FATAL] %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
	os.Exit(1)
}
