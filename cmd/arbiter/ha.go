package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb"
	"shinsakuto/pkg/models"
)

var raftNode *raft.Raft

type LogPayload struct {
	Action string          `json:"action"`
	Data   models.Downtime `json:"data"`
}

type ArbiterFSM struct{}

func (f *ArbiterFSM) Apply(l *raft.Log) interface{} {
	var p LogPayload
	json.Unmarshal(l.Data, &p)
	configMutex.Lock()
	defer configMutex.Unlock()

	switch p.Action {
	case "ADD_DT":
		downtimes = append(downtimes, p.Data)
		log.Printf("[HA] Replicated Downtime Added: %s", p.Data.ID)
	case "DEL_DT":
		var newList []models.Downtime
		for _, d := range downtimes {
			if d.ID != p.Data.ID { newList = append(newList, d) }
		}
		downtimes = newList
	}
	return nil
}

func (f *ArbiterFSM) Snapshot() (raft.FSMSnapshot, error) { return &ArbiterSnapshot{}, nil }
func (f *ArbiterFSM) Restore(rc io.ReadCloser) error {
	configMutex.Lock()
	defer configMutex.Unlock()
	return json.NewDecoder(rc).Decode(&downtimes)
}

type ArbiterSnapshot struct{}
func (s *ArbiterSnapshot) Persist(sink raft.SnapshotSink) error {
	json.NewEncoder(sink).Encode(downtimes)
	return sink.Close()
}
func (s *ArbiterSnapshot) Release() {}

func setupRaft() error {
	if !appConfig.HAEnabled { return nil }
	os.MkdirAll(appConfig.RaftDataDir, 0700)

	c := raft.DefaultConfig()
	c.LocalID = raft.ServerID(appConfig.RaftNodeID)
	if !appConfig.Debug { c.LogOutput = io.Discard }

	addr, _ := net.ResolveTCPAddr("tcp", appConfig.RaftBindAddr)
	transport, _ := raft.NewTCPTransport(appConfig.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)

	db, _ := boltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "raft.db"))
	stable, _ := boltdb.NewBoltStore(filepath.Join(appConfig.RaftDataDir, "stable.db"))
	fss, _ := raft.NewFileSnapshotStore(appConfig.RaftDataDir, 2, os.Stderr)

	r, err := raft.NewRaft(c, &ArbiterFSM{}, db, stable, fss, transport)
	if err != nil { return err }

	if appConfig.BootstrapCluster {
		r.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{{ID: c.LocalID, Address: transport.LocalAddr()}},
		})
	}
	raftNode = r
	return nil
}

func isLeader() bool {
	if !appConfig.HAEnabled { return true }
	return raftNode != nil && raftNode.State() == raft.Leader
}

func broadcastToFollowers() {
	if !isLeader() || !appConfig.HAEnabled { return }
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	filepath.Walk(appConfig.DefinitionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return nil }
		rel, _ := filepath.Rel(appConfig.DefinitionsDir, path)
		hdr, _ := tar.FileInfoHeader(info, "")
		hdr.Name = rel
		tw.WriteHeader(hdr)
		data, _ := os.ReadFile(path)
		tw.Write(data)
		return nil
	})
	tw.Close(); gzw.Close()

	for _, peer := range appConfig.ClusterNodes {
		url := fmt.Sprintf("http://%s/v1/cluster/sync-receiver", peer)
		http.Post(url, "application/x-gzip", bytes.NewReader(buf.Bytes()))
	}
}
