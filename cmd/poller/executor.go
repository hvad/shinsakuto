package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

// executeTask décode le JSON entrant pour identifier s'il s'agit d'un Host ou d'un Service
func executeTask(data []byte) models.CheckResult {
	// Structure temporaire pour extraire les champs communs
	var raw struct {
		ID           string `json:"id"`
		CheckCommand string `json:"check_command"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return models.CheckResult{Status: 3, Output: "Error decoding task JSON"}
	}

	logger.Info("[EXECUTOR] Running task ID: %s | Command: %s", raw.ID, raw.CheckCommand)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Exécution de la commande liée au service/hôte
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", raw.CheckCommand)
	output, err := cmd.CombinedOutput()

	result := models.CheckResult{
		ID:     raw.ID,
		Output: strings.TrimSpace(string(output)),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.Status = exitError.Sys().(syscall.WaitStatus).ExitStatus()
		} else {
			result.Status = 3 // UNKNOWN
			result.Output = err.Error()
		}
	} else {
		result.Status = 0 // OK
	}

	logger.Info("[RESULT] Task: %s | Status: %d | Output Snippet: %s", 
		result.ID, result.Status, result.Output)

	return result
}
