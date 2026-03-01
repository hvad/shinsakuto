package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shinsakuto/pkg/models" 
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

var (
	raftNode *raft.Raft
)

type LogPayload struct {
	Action string      `json:"action"`
	Data   interface{} `json:"data"`
}

func setupRaft() error {
    raftConfig := raft.DefaultConfig()
    raftConfig.LocalID = raft.ServerID(appConfig.RaftNodeID)
    // Accélérer les délais pour la détection locale
    raftConfig.ElectionTimeout = 500 * time.Millisecond
    raftConfig.HeartbeatTimeout = 500 * time.Millisecond

    addr, err := net.ResolveTCPAddr("tcp", appConfig.RaftBindAddr)
    if err != nil { return err }

    transport, err := raft.NewTCPTransport(appConfig.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)
    if err != nil { return err }

    // Correction "Rollback failed" : s'assurer que le chemin est unique par nœud
    if err := os.MkdirAll(appConfig.RaftDataDir, 0755); err != nil { return err }

    snapshots, _ := raft.NewFileSnapshotStore(appConfig.RaftDataDir, 2, os.Stderr)
    logStore, _ := raftboltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft-log.db"))
    stableStore, _ := raftboltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft-stable.db"))

    r, err := raft.NewRaft(raftConfig, &arbiterFSM{}, logStore, stableStore, snapshots, transport)
    if err != nil { return err }
    raftNode = r

    if appConfig.BootstrapCluster {
        log.Printf("[HA] Bootstrapping cluster...")
        configuration := raft.Configuration{
            Servers: []raft.Server{{ID: raftConfig.LocalID, Address: transport.LocalAddr()}},
        }
        raftNode.BootstrapCluster(configuration)
    } else {
        // Le nœud tente de rejoindre le cluster via les peers connus
        go joinCluster()
    }
    return nil
}

func joinCluster() {
    // Attendre que l'API du Leader soit potentiellement prête
    time.Sleep(2 * time.Second)
    for _, node := range appConfig.ClusterNodes {
        log.Printf("[HA] Tentative de jonction au cluster via : %s", node)
        url := fmt.Sprintf("http://%s/v1/cluster/join", node)
        body, _ := json.Marshal(map[string]string{
            "node_id": appConfig.RaftNodeID,
            "address": appConfig.RaftBindAddr,
        })
        resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
        if err == nil && resp.StatusCode == http.StatusOK {
            log.Printf("[HA] Cluster rejoint avec succès via %s", node)
            return
        }
    }
}

func isLeader() bool {
	if !appConfig.HAEnabled {
		return true
	}
	if raftNode == nil {
		return false
	}
	return raftNode.State() == raft.Leader
}

type arbiterFSM struct{}

func (f *arbiterFSM) Apply(l *raft.Log) interface{} {
	var p LogPayload
	if err := json.Unmarshal(l.Data, &p); err != nil {
		return err
	}

	switch p.Action {
	case "ADD_DT":
		dataBytes, _ := json.Marshal(p.Data)
		var d models.Downtime
		json.Unmarshal(dataBytes, &d)

		configMutex.Lock()
		downtimes = append(downtimes, d)
		configMutex.Unlock()
		
		log.Printf("[FSM] Downtime répliqué appliqué localement : %s", d.ID)
	}
	return nil
}

func (f *arbiterFSM) Snapshot() (raft.FSMSnapshot, error) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return &arbiterSnapshot{Downtimes: downtimes}, nil
}

func (f *arbiterFSM) Restore(rc io.ReadCloser) error {
	var s arbiterSnapshot
	if err := json.NewDecoder(rc).Decode(&s); err != nil {
		return err
	}
	configMutex.Lock()
	downtimes = s.Downtimes
	configMutex.Unlock()
	return nil
}

type arbiterSnapshot struct {
	Downtimes []models.Downtime
}

func (s *arbiterSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		if _, err := sink.Write(b); err != nil {
			return err
		}
		return sink.Close()
	}()
	if err != nil {
		sink.Cancel()
	}
	return err
}

func (s *arbiterSnapshot) Release() {}
