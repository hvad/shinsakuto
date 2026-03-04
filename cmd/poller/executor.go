package main

import (
	"context"
	"os/exec"
	"shinsakuto/pkg/models"
	"strings"
	"syscall"
	"time"
)

// executeTask handles command execution and captures Nagios-style exit codes
func executeTask(task models.CheckTask) models.CheckResult {
	logDebug("Initiating execution for task: %s", task.ID)

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
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			result.Status = 3 // UNKNOWN status
			result.Output = err.Error()
		}
	} else {
		result.Status = 0 // OK status
	}

	// Result is written to both Terminal and Log File
	logResult("ID: %s | Status: %d | Output: %s", result.ID, result.Status, result.Output)

	return result
}
