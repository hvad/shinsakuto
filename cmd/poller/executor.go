package main

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// executeTask handles command execution and captures Nagios-style exit codes
func executeTask(task models.CheckTask) models.CheckResult {
	// Debug trace for task initiation
	logger.Info("[EXECUTOR] Initiating execution for task: %s", task.ID)

	// Set a hard timeout of 30s for the check to prevent orphaned/hanging processes
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute via /bin/sh to support pipes and redirects in the command string
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", task.Command)
	output, err := cmd.CombinedOutput()

	result := models.CheckResult{
		ID:     task.ID,
		Output: strings.TrimSpace(string(output)),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Extract standard exit code (0: OK, 1: WARNING, 2: CRITICAL, 3: UNKNOWN)
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			// Handle non-exit errors (e.g., binary not found) as UNKNOWN
			result.Status = 3
			result.Output = err.Error()
		}
	} else {
		// Command executed successfully with exit code 0
		result.Status = 0
	}

	// Trace the final result status and output
	logger.Info("[RESULT] ID: %s | Status: %d | Output: %s", result.ID, result.Status, result.Output)

	return result
}
