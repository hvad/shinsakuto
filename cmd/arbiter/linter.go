package main

import (
	"fmt"
	"net"
	"shinsakuto/pkg/models"
)

// ObjectCounts stocke les statistiques des objets de supervision chargés et enregistrés.
type ObjectCounts struct {
	Hosts         int
	Services      int
	Commands      int
	TimePeriods   int
	Contacts      int
	HostGroups    int
	ServiceGroups int
}

// LinterResult contient le résultat de l'audit, incluant les erreurs, les alertes et les compteurs.
type LinterResult struct {
	Errors   []string
	Warnings []string
	Counts   ObjectCounts
}

// RunLinter exécute les vérifications sémantiques et compte les objets réels (Register != false).
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

	// Map pour vérifier l'existence des hôtes lors de la validation des services
	hostMap := make(map[string]bool)

	// 1. Validation et décompte des Hôtes
	for _, h := range cfg.Hosts {
		// On ignore les templates (Register: false) pour le décompte final et la validation réseau
		if h.Register != nil && !*h.Register {
			continue
		}

		res.Counts.Hosts++

		if h.ID == "" {
			res.Errors = append(res.Errors, "Hôte détecté avec un ID vide")
			continue
		}

		// Vérification des doublons d'ID
		if hostMap[h.ID] {
			res.Errors = append(res.Errors, fmt.Sprintf("ID d'hôte dupliqué : %s", h.ID))
		}
		hostMap[h.ID] = true

		// Validation basique de l'adresse IP/Hostname
		if h.Address != "" && h.Address != "localhost" && net.ParseIP(h.Address) == nil {
			// On génère un warning si l'adresse n'est ni une IP valide ni localhost
			res.Warnings = append(res.Warnings, fmt.Sprintf("L'hôte %s possède une adresse non standard : %s", h.ID, h.Address))
		}
	}

	// 2. Validation et décompte des Services
	for _, s := range cfg.Services {
		// On ignore les templates de services
		if s.Register != nil && !*s.Register {
			continue
		}

		res.Counts.Services++

		if s.ID == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Service lié à l'hôte %s sans ID", s.HostName))
		}

		// Vérification de la liaison Host <-> Service
		if s.HostName == "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Le service %s n'est rattaché à aucun hôte (host_name vide)", s.ID))
		} else if !hostMap[s.HostName] {
			res.Errors = append(res.Errors, fmt.Sprintf("Le service %s référence un hôte inconnu ou non enregistré : %s", s.ID, s.HostName))
		}
	}

	return res
}
