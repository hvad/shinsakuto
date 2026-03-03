package main

import (
	"fmt"
	"net"
	"shinsakuto/pkg/models"
)

// ObjectCounts stores statistics of loaded and registered monitoring objects.
// These counts are used for both the CLI report and the /v1/metrics endpoint.
type ObjectCounts struct {
	Hosts         int
	Services      int
	Commands      int
	TimePeriods   int
	Contacts      int
	HostGroups    int
	ServiceGroups int
}

// LinterResult contains the audit results, including critical errors, 
// non-blocking warnings, and the final object counts.
type LinterResult struct {
	Errors   []string
	Warnings []string
	Counts   ObjectCounts
}

// RunLinter executes semantic validation on the global configuration.
// It verifies the integrity of relationships between objects (e.g., Service -> Host)
// and updates the internal counters for the Arbiter.
func RunLinter(cfg *models.GlobalConfig) LinterResult {
	res := LinterResult{
		Counts: ObjectCounts{
			Commands:      len(cfg.Commands),
			TimePeriods:   len(cfg.TimePeriods),
			Contacts:      len(cfg.Contacts),
			HostGroups:    len(cfg.HostGroups),
			ServiceGroups: len(cfg.ServiceGroups),
		},
	}

	// hostMap stores registered host IDs to validate service attachments.
	// Templates (Register: false) are not added to this map.
	hostMap := make(map[string]bool)

	// 1. Host Validation and Counting
	for _, h := range cfg.Hosts {
		// Ignore templates as per Shinken-style inheritance logic
		if h.Register != nil && !*h.Register {
			continue
		}

		res.Counts.Hosts++

		// Critical: ID must not be empty for an active host
		if h.ID == "" {
			res.Errors = append(res.Errors, "Host definition found with an empty ID")
			continue
		}

		// Critical: Check for duplicate host IDs within the configuration
		if hostMap[h.ID] {
			res.Errors = append(res.Errors, fmt.Sprintf("Duplicate host ID detected: %s", h.ID))
		}
		hostMap[h.ID] = true

		// Warning: Basic network address validation (IPv4, IPv6, or 'localhost')
		if h.Address != "" && h.Address != "localhost" && net.ParseIP(h.Address) == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Host %s uses a non-standard address format: %s", h.ID, h.Address))
		}
	}

	// 2. Service Validation and Counting
	for _, s := range cfg.Services {
		// Ignore service templates
		if s.Register != nil && !*s.Register {
			continue
		}

		res.Counts.Services++

		// Critical: Service must have an ID
		if s.ID == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service attached to host %s is missing its ID", s.HostName))
		}

		// Critical: Verify Host relationship
		if s.HostName == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s is orphaned (no host_name assigned)", s.ID))
		} else if !hostMap[s.HostName] {
			// A service must point to a host that exists and is NOT a template
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s references an unknown or unregistered host: %s", s.ID, s.HostName))
		}
	}

	return res
}
