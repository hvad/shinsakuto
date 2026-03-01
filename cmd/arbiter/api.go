package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shinsakuto/pkg/models"
	"github.com/hashicorp/raft"
)

// startAPI initialise le serveur HTTP pour l'administration et la communication inter-nœuds.
func startAPI() {
	mux := http.NewServeMux()

	// Endpoint : État de santé et statistiques du nœud
	mux.HandleFunc("/v1/status", handleStatus)

	// Endpoint : Gestion des périodes de maintenance (Downtimes)
	mux.HandleFunc("/v1/downtime", handleDowntime)

	// Endpoint : Réception des fichiers de configuration (HA - Uniquement pour les Followers)
	mux.HandleFunc("/v1/cluster/sync-receiver", handleClusterSync)

	// Endpoint : Joindre le cluster
	mux.HandleFunc("/v1/cluster/join", handleJoin)

	addr := fmt.Sprintf("%s:%d", appConfig.APIAddress, appConfig.APIPort)
	log.Printf("[API] Serveur démarré sur %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[FATAL] Échec du serveur API : %v", err)
	}
}

// handleStatus retourne les informations sur le rôle du nœud et la dernière synchronisation.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()

	role := "Solo"
	if appConfig.HAEnabled && raftNode != nil {
		role = raftNode.State().String()
	}

	res := map[string]interface{}{
		"node_id":    appConfig.RaftNodeID,
		"role":       role,
		"last_sync":  lastSyncTime.Format(time.RFC3339),
		"sync_ok":    syncSuccess,
		"counts": map[string]int{
			"hosts":    len(currentConfig.Hosts),
			"services": len(currentConfig.Services),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// handleDowntime gère l'ajout et la consultation des maintenances.
func handleDowntime(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Seul le Leader peut accepter de nouveaux downtimes pour garantir la cohérence
		if !isLeader() {
			http.Error(w, "Échec : Ce nœud n'est pas le Leader Raft", http.StatusForbidden)
			return
		}

		var d models.Downtime
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, "JSON invalide", http.StatusBadRequest)
			return
		}

		d.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())

		// Si HA activé, on réplique via Raft. Sinon, on ajoute localement.
		if appConfig.HAEnabled && raftNode != nil {
			payload, _ := json.Marshal(LogPayload{Action: "ADD_DT", Data: d})
			raftNode.Apply(payload, 5*time.Second)
		} else {
			configMutex.Lock()
			downtimes = append(downtimes, d)
			configMutex.Unlock()
		}

		log.Printf("[API] Nouveau Downtime enregistré pour l'hôte : %s", d.HostName)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(d)

		// On déclenche une mise à jour immédiate vers le Scheduler
		go refreshConfig()
		return
	}

	// GET : Retourne la liste actuelle
	configMutex.RLock()
	defer configMutex.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(downtimes)
}

// handleClusterSync reçoit une archive TGZ du Leader et l'extrait localement.
func handleClusterSync(w http.ResponseWriter, r *http.Request) {
	if isLeader() || !appConfig.HAEnabled {
		http.Error(w, "Requête rejetée : Nœud Leader ou mode Solo", http.StatusForbidden)
		return
	}

	log.Println("[API] Réception d'une synchronisation de fichiers du Leader...")

	gzr, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, "Format GZIP invalide", http.StatusBadRequest)
		return
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Erreur TAR", http.StatusInternalServerError)
			return
		}

		target := filepath.Join(appConfig.DefinitionsDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				continue
			}
			io.Copy(f, tr)
			f.Close()
		}
	}

	w.WriteHeader(http.StatusOK)
	log.Println("[API] Synchronisation du cluster terminée avec succès.")

	// On rafraîchit la config locale pour prendre en compte les nouveaux fichiers
	go refreshConfig()
}

func handleJoin(w http.ResponseWriter, r *http.Request) {
    if !isLeader() {
        http.Error(w, "Not leader", http.StatusTemporaryRedirect)
        return
    }
    var req struct {
        NodeID  string `json:"node_id"`
        Address string `json:"address"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }
    log.Printf("[HA] Ajout du nœud %s (%s) au cluster", req.NodeID, req.Address)
    future := raftNode.AddVoter(raft.ServerID(req.NodeID), raft.ServerAddress(req.Address), 0, 0)
    if err := future.Error(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
}
