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

	QueryChainSync = "chain.sync"
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

func (p Cluster) ChainFrom(nodeID string) (*blocks.Blockchain, error) {
	resp, err := p.serf.Query(QueryChainSync, []byte{}, &serf.QueryParam{FilterNodes: []string{nodeID}})
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	chain := &blocks.Blockchain{}
	for ns := range resp.ResponseCh() {
		fmt.Printf("got payload: %x", ns.Payload)
		//just read the first one and return it
		if err := json.Unmarshal(ns.Payload, chain); err != nil {
			return nil, err
		}
		break
	}
	return chain, nil
}

type BlockEvent struct {
	EventType string        `json:"event"`
	Block     *blocks.Block `json:"block"`
	NodeID    string        `json:"node_id"`
}
