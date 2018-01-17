package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/blocks"
)

func NewTransferManager(chain *blocks.Blockchain) *TransferManager {
	return &TransferManager{
		chain:       chain,
		exit:        make(chan bool, 1),
		errors:      make(chan error, 1000),
		connections: make(chan net.Conn, 100),
	}
}

type TransferManager struct {
	chain       *blocks.Blockchain
	errors      chan error
	connections chan net.Conn
	exit        chan bool
}

func (t *TransferManager) FetchChain(fromNode *serf.Member) error {

	port, ok := fromNode.Tags["transfer.port"]
	if !ok {
		return fmt.Errorf("target host does not advertise a tranmsfer port")
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", fromNode.Addr.String(), port))
	if err != nil {
		return errors.Wrap(err, "failed to open connection to target host")
	}
	defer conn.Close()

	chain := &blocks.Blockchain{}
	if err := json.NewDecoder(conn).Decode(chain); err != nil {
		return err
	}

	return t.chain.Replace(chain)
}

func (t *TransferManager) Listen(transferAddr string) error {

	ln, err := net.Listen("tcp", transferAddr)
	if err != nil {
		return err
	}
	defer func() {
		ln.Close()
		close(t.errors)
	}()

	go func() {
		for {
			select {
			case <-t.exit:
				return
			case conn := <-t.connections:
				go func() {
					defer conn.Close()
					if err := json.NewEncoder(conn).Encode(t.chain.Snapshot()); err != nil {
						t.errors <- err
					}
					return
				}()
			case err := <-t.errors:
				log.Printf("error in transfer listener: %s", err.Error())
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			t.errors <- err
			continue
		}
		t.connections <- conn
	}
}

func (t *TransferManager) Close() {
	t.exit <- true
}
