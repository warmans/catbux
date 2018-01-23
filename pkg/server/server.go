package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/blocks"
)

func New(APIAddr string, chain *blocks.Blockchain, cluster *Cluster, tm *TransferManager) *Server {
	return &Server{addr: APIAddr, chain: chain, cluster: cluster, tm: tm}
}

type Server struct {
	addr    string
	chain   *blocks.Blockchain
	cluster *Cluster
	tm      *TransferManager
}

func (s *Server) Start() error {

	http.Handle("/blocks", http.HandlerFunc(s.handleBlocks))
	http.Handle("/mine", http.HandlerFunc(s.handleMine))
	http.Handle("/peers", http.HandlerFunc(s.handlePeers))

	//initial sync
	if err := s.syncChain(); err != nil {
		log.Println("Failed initial chain sync: " + err.Error())
	}

	go func() {
		s.processEvents()
	}()

	go func() {
		log.Printf("Http started on %s", s.addr)
		err := http.ListenAndServe(s.addr, nil)
		if err != nil {
			log.Fatalf("HTTP server failed: %s", err)
		}
	}()

	return nil
}

func (s *Server) handleBlocks(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(s.chain.Snapshot())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleMine(w http.ResponseWriter, r *http.Request) {

	//create a new block to be mined
	newBlock := &blocks.Block{
		Index:     int64(s.chain.Len()),
		PrevHash:  s.chain.Last().Hash,
		Timestamp: time.Now(),
		//Data:       string(data),
		Difficulty: s.chain.GetCurrentDifficulty(),
	}

	//mine the block + keep the hash in line with the block content
	if err := blocks.FindNonce(newBlock); err != nil {
		http.Error(w, errors.Wrap(err, "failed finding nonce").Error(), http.StatusInternalServerError)
		return
	}

	if err := s.chain.Append(newBlock); err != nil {
		http.Error(w, errors.Wrap(err, "node found but could not be appended to chain").Error(), http.StatusInternalServerError)
		return
	}

	if err := s.cluster.Broadcast(&BlockEvent{EventNewBlock, newBlock, s.cluster.serf.LocalMember().Name}); err != nil {
		log.Printf("failed to broadcast new block: %s", err.Error())
	}
	s.handleBlocks(w, r)
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(s.cluster.Peers())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) processEvents() {
	for e := range s.cluster.Events {
		switch e.EventType() {
		case serf.EventUser:
			ue := e.(serf.UserEvent)
			if ue.Name == EventNewBlock {
				if err := s.processNewBlockEv(ue); err != nil {
					log.Printf("error handling new block event: %s", err.Error())
					return
				}
			}
		case serf.EventQuery:
			qe := e.(*serf.Query)
			log.Printf("got a query request: %s", qe.Name)
		}
	}
}

func (s *Server) processNewBlockEv(ue serf.UserEvent) error {
	blockEv := &BlockEvent{}
	if err := json.Unmarshal(ue.Payload, blockEv); err != nil {
		return errors.Wrap(err, "decode failed")
	}
	if blockEv.NodeID == s.cluster.serf.LocalMember().Name {
		return nil
	}
	expectedNextBlockIdx := s.chain.Last().Index + 1
	if blockEv.Block.Index == expectedNextBlockIdx {
		if err := s.chain.Append(blockEv.Block); err != nil {
			return errors.Wrap(err, "append failed")
		}
	}
	if blockEv.Block.Index > expectedNextBlockIdx {
		log.Println("require full sync from " + blockEv.NodeID)
		if peer := s.cluster.GetPeer(blockEv.NodeID); peer != nil {
			if err := s.syncChainFrom(peer); err != nil {
				return errors.Wrap(err, "syncing chain failed")
			}
			log.Println("sync OK")
		}
	}
	return nil
}

func (s *Server) syncChain() error {
	peers := s.cluster.Peers()
	if len(peers) == 0 {
		return nil //nobody to sync from... I guess I'm the first node
	}
	var peer *serf.Member = nil
	for _, p := range peers {
		if p.Name != s.cluster.serf.LocalMember().Name {
			peer = &p
			break
		}
	}
	if peer == nil {
		return nil //no peers
	}
	return s.syncChainFrom(peer)

}

func (s *Server) syncChainFrom(peer *serf.Member) error {
	log.Printf("syncing chain from %s", peer.Name)
	return s.tm.FetchChain(peer)
}
