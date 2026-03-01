package main

import (
	"testing"
	"shinsakuto/pkg/models"
)

func TestRunLinter(t *testing.T) {
	// Test Case 1: Duplicate Host IDs
	t.Run("Duplicate Host IDs", func(t *testing.T) {
		cfg := &models.GlobalConfig{
			Hosts: []models.Host{
				{ID: "server-1", Address: "127.0.0.1"},
				{ID: "server-1", Address: "192.168.1.1"},
			},
		}
		res := RunLinter(cfg)
		if len(res.Errors) == 0 {
			t.Errorf("Expected error for duplicate Host ID, got none")
		}
	})

	// Test Case 2: Unknown Host Reference in Service
	t.Run("Unknown Host Reference", func(t *testing.T) {
		cfg := &models.GlobalConfig{
			Hosts: []models.Host{
				{ID: "server-1", Address: "127.0.0.1"},
			},
			Services: []models.Service{
				{ID: "http-check", HostName: "server-99", CheckCommand: "check_http"},
			},
		}
		res := RunLinter(cfg)
		found := false
		for _, err := range res.Errors {
			if err == "Service http-check references unknown host: server-99" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error for unknown host reference, but it was not found in: %v", res.Errors)
		}
	})

	// Test Case 3: Invalid IP Address Warning
	t.Run("Invalid IP Warning", func(t *testing.T) {
		cfg := &models.GlobalConfig{
			Hosts: []models.Host{
				{ID: "server-1", Address: "999.999.999.999"},
			},
		}
		res := RunLinter(cfg)
		if len(res.Warnings) == 0 {
			t.Errorf("Expected warning for invalid IP address, got none")
		}
	})
}
