package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// executeTask runs the command-line, replaces the $ADDRESS$ macro, 
// and handles custom user macros like $USER1$.
func executeTask(data []byte) models.CheckResult {
	// Internal struct updated to include the Macros map from the Scheduler.
	var task struct {
		ID          string            `json:"id"`
		CommandLine string            `json:"command_line"`
		Address     string            `json:"address"`
		Macros      map[string]string `json:"macros"` 
	}

	if err := json.Unmarshal(data, &task); err != nil {
		return models.CheckResult{
			Status: 3, // UNKNOWN
			Output: "Error decoding task JSON: " + err.Error(),
		}
	}

	finalCommand := task.CommandLine

	// 1. Perform custom macro substitution (e.g., $USER1$, $TIMEOUT$).
	// We iterate through the macros map and replace each occurrence in the command string.
	for key, value := range task.Macros {
		macroName := fmt.Sprintf("$%s$", key)
		finalCommand = strings.ReplaceAll(finalCommand, macroName, value)
	}

	// 2. Perform standard macro substitution for $ADDRESS$.
	finalCommand = strings.ReplaceAll(finalCommand, "$ADDRESS$", task.Address)

	logger.Info("[EXECUTOR] Running task ID: %s | Command: %s", task.ID, finalCommand)

	// Set a 30s timeout for safety.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute via shell to support variables and complex arguments.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", finalCommand)
	
	// Inject standard binary paths to avoid 'command not found' (Error 127).
	cmd.Env = append(os.Environ(), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

	output, err := cmd.CombinedOutput()

	result := models.CheckResult{
		ID:     task.ID,
		Output: strings.TrimSpace(string(output)),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Extract Nagios-style exit codes (0:OK, 1:WARN, 2:CRIT, 3:UNK).
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			// Handle context timeout or execution failure.
			result.Status = 3 
			result.Output = err.Error()
		}
	} else {
		// Command finished successfully (Exit Code 0).
		result.Status = 0 
	}

	logger.Info("[RESULT] Task: %s | Status: %d | Output: %s", 
		result.ID, result.Status, result.Output)

	return result
}
