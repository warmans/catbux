package server

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/hashicorp/serf/serf"
	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/blocks"
)

func New(APIAddr string, chain *blocks.Blockchain, cluster *Cluster) *Server {
	return &Server{addr: APIAddr, chain: chain, cluster: cluster}
}

type Server struct {
	addr    string
	chain   *blocks.Blockchain
	cluster *Cluster
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
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	block, err := s.chain.NewBlock(string(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.cluster.Broadcast(&BlockEvent{EventNewBlock, block, s.cluster.serf.LocalMember().Name}); err != nil {
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
			log.Printf("got a query request")
			qe := e.(*serf.Query)
			if qe.Name == QueryChainSync {
				b, err := json.Marshal(s.chain.Snapshot())
				if err != nil {
					log.Printf("error sending chain shapshot: %s", err.Error())
					return
				}
				qe.Respond(b)
			}
		}
	}
}

func (s *Server) processNewBlockEv(ue serf.UserEvent) error {
	blockEv := &BlockEvent{}
	if err := json.Unmarshal(ue.Payload, blockEv); err != nil {
		return errors.Wrap(err, "decode failed")
	}
	expectedNextBlockIdx := s.chain.Last().Index + 1
	if blockEv.Block.Index == expectedNextBlockIdx {
		if err := s.chain.Append(blockEv.Block); err != nil {
			return errors.Wrap(err, "append failed")
		}
	}
	if blockEv.Block.Index > expectedNextBlockIdx {
		log.Println("require full sync ")
		if err := s.syncChainFrom(blockEv.NodeID); err != nil {
			return errors.Wrap(err, "syncing chain failed")
		}
	}
	return nil
}

//todo: sync doesn't really work since the max message size is small

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
	log.Printf("syncing chain from %s", peer.Name)
	return s.syncChainFrom(peer.Name)

}

func (s *Server) syncChainFrom(fromNode string) error {
	chain, err := s.cluster.ChainFrom(fromNode)
	if err != nil {
		return err
	}
	return s.chain.Replace(chain)
}
