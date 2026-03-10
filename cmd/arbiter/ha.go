package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shinsakuto/pkg/logger"
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

	raftConfig.ElectionTimeout = 3000 * time.Millisecond
	raftConfig.HeartbeatTimeout = 1000 * time.Millisecond
	raftConfig.LeaderLeaseTimeout = 500 * time.Millisecond

	addr, err := net.ResolveTCPAddr("tcp", appConfig.RaftBindAddr)
	if err != nil {
		return fmt.Errorf("Failed to resolve raft bind address: %v", err)
	}

	transport, err := raft.NewTCPTransport(appConfig.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return fmt.Errorf("Failed to create raft transport: %v", err)
	}

	if err := os.MkdirAll(appConfig.RaftDataDir, 0755); err != nil {
		return fmt.Errorf("Failed to create raft data directory: %v", err)
	}

	dbPath := filepath.Join(appConfig.RaftDataDir, "raft-log.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) && appConfig.BootstrapCluster {
		logger.Always("[HA] Initial bootstrap detected, preparing clean storage in %s", appConfig.RaftDataDir)
	}

	snapshots, _ := raft.NewFileSnapshotStore(appConfig.RaftDataDir, 2, os.Stderr)
	logStore, _ := raftboltdb.NewBoltStore(dbPath)
	stableStore, _ := raftboltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft-stable.db"))

	r, err := raft.NewRaft(raftConfig, &arbiterFSM{}, logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("Failed to start raft: %v", err)
	}
	raftNode = r

	hasState, _ := raft.HasExistingState(logStore, stableStore, snapshots)

	if appConfig.BootstrapCluster && !hasState {
		logger.Always("[HA] No existing state found. Bootstrapping as Leader: %s", raftConfig.LocalID)
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	} else if !appConfig.BootstrapCluster {
		logger.Always("[HA] Node %s starting as Follower, waiting to join...", raftConfig.LocalID)
		go joinCluster()
	}

	return nil
}

func joinCluster() {
	time.Sleep(5 * time.Second)

	for _, nodeAddr := range appConfig.ClusterNodes {
		url := fmt.Sprintf("http://%s/v1/cluster/join", nodeAddr)
		payload, _ := json.Marshal(map[string]string{
			"node_id": appConfig.RaftNodeID,
			"address": appConfig.RaftBindAddr,
		})

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
		if err == nil && resp.StatusCode == http.StatusOK {
			logger.Always("[HA] Successfully joined cluster via %s", nodeAddr)
			return
		}
		if err != nil {
			logger.Always("[HA] Join attempt failed for %s: %v", nodeAddr, err)
		}
	}
}

func isLeader() bool {
	if !appConfig.HAEnabled { return true }
	return raftNode != nil && raftNode.State() == raft.Leader
}

type arbiterFSM struct{}

func (f *arbiterFSM) Apply(l *raft.Log) interface{} {
	var p LogPayload
	json.Unmarshal(l.Data, &p)
	if p.Action == "ADD_DT" {
		var d models.Downtime
		dataBytes, _ := json.Marshal(p.Data)
		json.Unmarshal(dataBytes, &d)
		configMutex.Lock()
		downtimes = append(downtimes, d)
		configMutex.Unlock()
		logger.Info("[FSM] Replicated downtime applied: %s", d.ID)
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
	if err := json.NewDecoder(rc).Decode(&s); err != nil { return err }
	configMutex.Lock()
	downtimes = s.Downtimes
	configMutex.Unlock()
	return nil
}

type arbiterSnapshot struct { Downtimes []models.Downtime }
func (s *arbiterSnapshot) Persist(sink raft.SnapshotSink) error {
	b, _ := json.Marshal(s)
	sink.Write(b)
	return sink.Close()
}
func (s *arbiterSnapshot) Release() {}
