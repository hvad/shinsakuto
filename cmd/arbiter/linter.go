package main

import (
	"fmt"
	"net"
	"shinsakuto/pkg/models"
)

// ObjectCounts stores statistics of loaded and registered monitoring objects.
type ObjectCounts struct {
	Hosts         int
	Services      int
	Commands      int
	TimePeriods   int
	Contacts      int
	HostGroups    int
	ServiceGroups int
}

// LinterResult contains the audit results, including errors, warnings, and counts.
type LinterResult struct {
	Errors   []string
	Warnings []string
	Counts   ObjectCounts
}

// RunLinter performs semantic checks and counts active objects (Register != false).
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

	// Host lookup map for service validation
	hostMap := make(map[string]bool)

	// 1. Host Validation and Counting
	for _, h := range cfg.Hosts {
		// Skip templates (Register: false) for counts and network validation
		if h.Register != nil && !*h.Register {
			continue
		}

		res.Counts.Hosts++

		if h.ID == "" {
			res.Errors = append(res.Errors, "Host detected with an empty ID")
			continue
		}

		// Duplicate ID check
		if hostMap[h.ID] {
			res.Errors = append(res.Errors, fmt.Sprintf("Duplicate host ID detected: %s", h.ID))
		}
		hostMap[h.ID] = true

		// Basic IP/Hostname validation
		if h.Address != "" && h.Address != "localhost" && net.ParseIP(h.Address) == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Host %s uses a non-standard address format: %s", h.ID, h.Address))
		}
	}

	// 2. Service Validation and Counting
	for _, s := range cfg.Services {
		if s.Register != nil && !*s.Register {
			continue
		}

		res.Counts.Services++

		if s.ID == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service linked to host %s has no ID", s.HostName))
		}

		// Validation of Host <-> Service relationship
		if s.HostName == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s is not attached to any host (host_name is empty)", s.ID))
		} else if !hostMap[s.HostName] {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s references an unknown or unregistered host: %s", s.ID, s.HostName))
		}
	}

	return res
}
