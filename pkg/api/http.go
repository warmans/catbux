package api

import (
	"net/http"
	"encoding/json"
	"net"
	"log"
	"io/ioutil"
	"github.com/warmans/catbux/pkg/blocks"
)

func New(addr string, chain *blocks.Blockchain) *API {
	return &API{addr: addr, chain: chain}
}

type API struct {
	addr string
	ln   net.Listener
	chain *blocks.Blockchain
}

func (s *API) Start() error {

	http.Handle("/blocks", http.HandlerFunc(s.handleBlocks))
	http.Handle("/mine", http.HandlerFunc(s.handleMine))

	go func() {
		err := http.ListenAndServe(s.addr, nil)
		if err != nil {
			log.Fatalf("HTTP server failed: %s", err)
		}
	}()
	return nil
}

// Close closes the service.
func (s *API) Close() {
	s.ln.Close()
	return
}

func (s *API) handleBlocks(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(s.chain.Snapshot())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *API) handleMine(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.chain.Append(string(data)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleBlocks(w, r)
}
