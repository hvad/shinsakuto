package main

import (
	"fmt"
	"net"
	"shinsakuto/pkg/logger"
	"shinsakuto/pkg/models"
)

type ObjectCounts struct {
	Hosts         int
	Services      int
	Commands      int
	TimePeriods   int
	Contacts      int
	HostGroups    int
	ServiceGroups int
}

type LinterResult struct {
	Errors   []string
	Warnings []string
	Counts   ObjectCounts
}

func RunLinter(cfg *models.GlobalConfig) LinterResult {
	logger.Info("[LINTER] Starting configuration audit.")
	res := LinterResult{
		Counts: ObjectCounts{
			Commands:      len(cfg.Commands),
			TimePeriods:   len(cfg.TimePeriods),
			Contacts:      len(cfg.Contacts),
			HostGroups:    len(cfg.HostGroups),
			ServiceGroups: len(cfg.ServiceGroups),
		},
	}

	hostMap := make(map[string]bool)

	for _, h := range cfg.Hosts {
		if h.Register != nil && !*h.Register {
			continue
		}

		res.Counts.Hosts++

		if h.ID == "" {
			res.Errors = append(res.Errors, "Host definition found with an empty ID")
			continue
		}

		if hostMap[h.ID] {
			res.Errors = append(res.Errors, fmt.Sprintf("Duplicate host ID detected: %s", h.ID))
		}
		hostMap[h.ID] = true

		if h.Address != "" && h.Address != "localhost" && net.ParseIP(h.Address) == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Host %s uses a non-standard address format: %s", h.ID, h.Address))
		}
	}

	for _, s := range cfg.Services {
		if s.Register != nil && !*s.Register {
			continue
		}

		res.Counts.Services++

		if s.ID == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service attached to host %s is missing its ID", s.HostName))
		}

		if s.HostName == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s is orphaned (no host_name assigned)", s.ID))
		} else if !hostMap[s.HostName] {
			res.Errors = append(res.Errors, fmt.Sprintf("Service %s references an unknown or unregistered host: %s", s.ID, s.HostName))
		}
	}

	logger.Info("[LINTER] Audit complete: %d errors, %d warnings", len(res.Errors), len(res.Warnings))
	return res
}
