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

// executeTask runs the provided command and captures Nagios-style exit codes
func executeTask(task models.CheckTask) models.CheckResult {
	// Debug trace for start of execution
	logger.Info("[EXECUTOR] Running task ID: %s | Command: %s", task.ID, task.Command)

	// Set a 30s context timeout to ensure no hanging processes or orphans
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute through /bin/sh to support shell features in the command string
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", task.Command)
	output, err := cmd.CombinedOutput()

	result := models.CheckResult{
		ID:     task.ID,
		Output: strings.TrimSpace(string(output)),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Extract standard Nagios exit codes: 
			// 0: OK, 1: WARNING, 2: CRITICAL, 3: UNKNOWN
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			// If execution itself failed (e.g. context timeout or binary not found)
			result.Status = 3 // UNKNOWN
			result.Output = err.Error()
		}
	} else {
		// Command finished successfully with exit code 0
		result.Status = 0 // OK
	}

	// Trace final result for debugging
	logger.Info("[RESULT] Task: %s | Status: %d | Output Snippet: %s", 
		result.ID, result.Status, result.Output)

	return result
}
