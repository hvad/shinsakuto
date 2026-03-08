package main

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"shinsakuto/pkg/models"
	"shinsakuto/pkg/logger"
)

// executeTask handles command execution and captures Nagios-style exit codes
func executeTask(task models.CheckTask) models.CheckResult {
	logger.Info("[EXECUTOR] Initiating execution for task: %s", task.ID)

	// Set a hard timeout for the check to prevent orphaned processes
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
			result.Status = 3 // UNKNOWN status
			result.Output = err.Error()
		}
	} else {
		result.Status = 0 // OK status
	}

	logger.Info("[RESULT] ID: %s | Status: %d | Output: %s", result.ID, result.Status, result.Output)

	return result
}
