package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// executeTask runs the command-line and replaces the $ADDRESS$ macro
func executeTask(data []byte) models.CheckResult {
	// Internal struct to match the JSON sent by popTaskHandler
	var task struct {
		ID          string `json:"id"`
		CommandLine string `json:"command_line"`
		Address     string `json:"address"`
	}

	if err := json.Unmarshal(data, &task); err != nil {
		return models.CheckResult{
			Status: 3, // UNKNOWN
			Output: "Error decoding task JSON: " + err.Error(),
		}
	}

	// Perform macro substitution for $ADDRESS$
	finalCommand := strings.ReplaceAll(task.CommandLine, "$ADDRESS$", task.Address)

	logger.Info("[EXECUTOR] Running task ID: %s | Command: %s", task.ID, finalCommand)

	// Set a 30s timeout for safety
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute via shell to support variables and complex arguments
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", finalCommand)
	
	// Inject standard binary paths to avoid 'command not found' (Error 127)
	cmd.Env = append(os.Environ(), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

	output, err := cmd.CombinedOutput()

	result := models.CheckResult{
		ID:     task.ID,
		Output: strings.TrimSpace(string(output)),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Extract Nagios exit codes (0:OK, 1:WARN, 2:CRIT, 3:UNK)
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			// Context timeout or execution failure
			result.Status = 3 
			result.Output = err.Error()
		}
	} else {
		// Command finished successfully
		result.Status = 0 
	}

	logger.Info("[RESULT] Task: %s | Status: %d | Output: %s", 
		result.ID, result.Status, result.Output)

	return result
}
