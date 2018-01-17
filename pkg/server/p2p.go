package server

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/blocks"
)

const (
	EventNewBlock = "block.new"
)

func NewCluster(config *serf.Config, seedNodes ...string) (*Cluster, error) {

	events := make(chan serf.Event, 1000)
	config.EventCh = events

	cluster, err := serf.Create(config)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't create cluster")
	}

	if len(seedNodes) > 0 {
		if _, err := cluster.Join(seedNodes, true); err != nil {
			return nil, fmt.Errorf("Couldn't join cluster: %v\n", err)
		}
	}
	c := &Cluster{serf: cluster, Events: events}
	return c, nil
}

type Cluster struct {
	serf   *serf.Serf
	Events chan serf.Event
}

func (p *Cluster) Broadcast(ev *BlockEvent) error {
	data := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(data).Encode(ev); err != nil {
		return err
	}
	return p.serf.UserEvent(ev.EventType, data.Bytes(), false)
}

func (p *Cluster) Close() error {
	err := p.serf.Leave()
	close(p.Events)
	return err
}

func (p *Cluster) Peers() []serf.Member {
	return p.serf.Members()
}

func (p *Cluster) GetPeer(nodeID string) *serf.Member {
	for _, m := range p.serf.Members() {
		if m.Name == nodeID {
			return &m
		}
	}
	return nil
}

type BlockEvent struct {
	EventType string        `json:"event"`
	Block     *blocks.Block `json:"block"`
	NodeID    string        `json:"node_id"`
}
