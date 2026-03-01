package main

import (
	"fmt"
	"net"
	"shinsakuto/pkg/models"
)

// ObjectCounts holds the statistics of loaded supervision objects.
type ObjectCounts struct {
	Hosts         int
	Services      int
	Commands      int
	TimePeriods   int
	Contacts      int
	HostGroups    int
	ServiceGroups int
}

// LinterResult contains the audit results.
type LinterResult struct {
	Errors   []string
	Warnings []string
	Counts   ObjectCounts
}

// RunLinter performs semantic checks and counts objects.
func RunLinter(cfg *models.GlobalConfig) LinterResult {
	res := LinterResult{
		Counts: ObjectCounts{
			Hosts:         len(cfg.Hosts),
			Services:      len(cfg.Services),
			Commands:      len(cfg.Commands),
			TimePeriods:   len(cfg.TimePeriods),
			Contacts:      len(cfg.Contacts),
			HostGroups:    len(cfg.HostGroups),
			ServiceGroups: len(cfg.ServiceGroups),
		},
	}

	// 1. Host Validation (ID & IP)
	hostMap := make(map[string]bool)
	for _, h := range cfg.Hosts {
		if h.ID == "" {
			res.Errors = append(res.Errors, "Host found with empty ID")
			continue
		}
		if hostMap[h.ID] {
			res.Errors = append(res.Errors, fmt.Sprintf("Duplicate Host ID detected: %s", h.ID))
		}
		hostMap[h.ID] = true

		if h.Address != "" && net.ParseIP(h.Address) == nil && h.Address != "localhost" {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Host %s has a non-standard address: %s", h.ID, h.Address))
		}
	}

	// 2. Service Validation (Host Reference)
	for _, s := range cfg.Services {
		if s.ID == "" {
			res.Errors = append(res.Errors, "Service found with empty ID")
		}
		if !hostMap[s.HostName] {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s references unknown host: %s", s.ID, s.HostName))
		}
	}

	return res
}
