package api

import (
	"net/http"
	"encoding/json"
	"github.com/warmans/trinkgeld/pkg/node"
	"net"
	"log"
)

func New(addr string, node *node.Node) *API {
	return &API{addr: addr, node: node}
}

type API struct {
	addr string
	node *node.Node
	ln   net.Listener
}

func (s *API) Start() error {

	http.Handle("/join", http.HandlerFunc(s.handleJoin))

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

func (s *API) handleJoin(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(m) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	remoteAddr, ok := m["addr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	nodeID, ok := m["id"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.node.Join(nodeID, remoteAddr); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
