package node

import (
	"github.com/hashicorp/raft"
	"time"
	"os"
	"net"
	"fmt"
	"github.com/hashicorp/raft-boltdb"
	"path/filepath"
	"github.com/warmans/trinkgeld/pkg/blocks"
	"encoding/json"
	"io"
	"log"
)

const (
	retainSnapshotCount = 2
	raftTimeout         = 10 * time.Second
)

func New(raftDir, raftBind string) *Node {
	return &Node{
		RaftDir:  raftDir,
		RaftBind: raftBind,
		chain: blocks.NewBlockchain(),
	}
}

type Node struct {
	RaftDir  string
	RaftBind string
	raft     *raft.Raft
	chain    *blocks.Blockchain
}

func (n *Node) Chain() *blocks.Blockchain {
	return n.chain
}

func (n *Node) Broadcast() error {
	b, err := json.Marshal(n.chain)
	if err != nil {
		return err
	}
	f := n.raft.Apply(b, raftTimeout)
	return f.Error()
}

func (n *Node) Join(nodeID, addr string) error {
	log.Printf("received join request for remote node %s at %s", nodeID, addr)
	f := n.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	if f.Error() != nil {
		return f.Error()
	}
	log.Printf("node %s at %s joined successfully", nodeID, addr)
	return nil
}

func (n *Node) Start(nodeID string, enableSingle bool) error {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	addr, err := net.ResolveTCPAddr("tcp", n.RaftBind)
	if err != nil {
		return err
	}
	transport, err := raft.NewTCPTransport(n.RaftBind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}

	snapshots, err := raft.NewFileSnapshotStore(n.RaftDir, retainSnapshotCount, os.Stderr)
	if err != nil {
		return fmt.Errorf("file snapshot store: %s", err)
	}

	// Create the log store and stable store.
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(n.RaftDir, "raft.db"))
	if err != nil {
		return fmt.Errorf("new bolt store: %s", err)
	}

	ra, err := raft.NewRaft(config, (*fsm)(n), logStore, logStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %s", err)
	}
	n.raft = ra

	if enableSingle {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		ra.BootstrapCluster(configuration)
	}
	return nil
}

type fsm Node

// Apply applies a Raft log entry to the key-value store.
func (f *fsm) Apply(l *raft.Log) interface{} {
	var c *blocks.Blockchain
	if err := json.Unmarshal(l.Data, &c); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}
	return f.chain.Replace(c)
}

// Snapshot returns a snapshot of the key-value store.
func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	chain := *f.chain
	return &fsmSnapshot{chain: &chain}, nil
}

// Restore stores the key-value store to a previous state.
func (f *fsm) Restore(rc io.ReadCloser) error {
	c := &blocks.Blockchain{}
	if err := json.NewDecoder(rc).Decode(c); err != nil {
		return err
	}
	// Set the state from the snapshot, no lock required according to
	// Hashicorp docs.
	f.chain = c
	return nil
}

type fsmSnapshot struct {
	chain *blocks.Blockchain
}

func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode data.
		b, err := json.Marshal(f.chain)
		if err != nil {
			return err
		}

		// Write data to sink.
		if _, err := sink.Write(b); err != nil {
			return err
		}

		// Close the sink.
		return sink.Close()
	}()

	if err != nil {
		sink.Cancel()
	}

	return err
}

func (f *fsmSnapshot) Release() {}
