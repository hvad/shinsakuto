package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

var raftNode *raft.Raft

type RaftPayload struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Value  interface{} `json:"value"`
}

// setupRaft configures the high availability cluster
func setupRaft() error {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(appConfig.RaftNodeID)

	addr, err := net.ResolveTCPAddr("tcp", appConfig.RaftBindAddr)
	if err != nil { return err }

	transport, err := raft.NewTCPTransport(appConfig.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil { return err }

	os.MkdirAll(appConfig.RaftDataDir, 0755)
	snapshots, _ := raft.NewFileSnapshotStore(appConfig.RaftDataDir, 2, os.Stderr)
	logStore, _ := raftboltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft-log.db"))
	stableStore, _ := raftboltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft-stable.db"))

	r, err := raft.NewRaft(config, &reactionnerFSM{}, logStore, stableStore, snapshots, transport)
	if err != nil { return err }
	raftNode = r

	if appConfig.BootstrapCluster {
		raftNode.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{{ID: config.LocalID, Address: transport.LocalAddr()}},
		})
	}
	return nil
}

// isLeader checks if the current node is the active Raft leader
func isLeader() bool {
	if !appConfig.HAEnabled { return true }
	return raftNode != nil && raftNode.State() == raft.Leader
}

type reactionnerFSM struct{}

func (f *reactionnerFSM) Apply(l *raft.Log) interface{} {
	var p RaftPayload
	json.Unmarshal(l.Data, &p)
	mu.Lock()
	defer mu.Unlock()

	switch p.Action {
	case "ACK":
		acknowledgments[p.ID] = true
	case "RECOVERY":
		delete(acknowledgments, p.ID)
	case "MAINT":
		maintenances[p.ID] = int64(p.Value.(float64))
	}
	return nil
}

func (f *reactionnerFSM) Snapshot() (raft.FSMSnapshot, error) { return &fsmSnapshot{}, nil }
func (f *reactionnerFSM) Restore(rc io.ReadCloser) error { return nil }

type fsmSnapshot struct{}
func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error { return nil }
func (s *fsmSnapshot) Release() {}
