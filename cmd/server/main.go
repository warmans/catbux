package main

//based on https://github.com/otoolep/hraftd and http://lhartikk.github.io/jekyll/update/2017/07/14/chapter1.html

import (
	"flag"
	"os"
	"log"
	"os/signal"
	"net"
	"github.com/warmans/catbux/pkg/api"
	"github.com/satori/go.uuid"
	"github.com/warmans/catbux/pkg/blocks"
	"github.com/hashicorp/memberlist"
	"strings"
)

const (
	DefaultHTTPBind = "localhost:8686"
)

var (
	httpBind     = flag.String("http-bind", DefaultHTTPBind, "Set the HTTP bind address")
	seedNodeList = flag.String("seed-nodes", "", "address of an existing cluster node")
	nodeID       = flag.String("id", "", "Node ID (leaving blank will generate one)")
)

func main() {
	flag.Parse()

	if *nodeID == "" {
		*nodeID = mustGetNodeID()
	}

	seedNodes := []string{}
	if *seedNodeList != "" {
		seedNodes = strings.Split(*seedNodeList, ",")
	}

	conf := memberlist.DefaultLocalConfig()
	conf.Name = *nodeID
	conf.BindPort = mustGetFreePort()
	conf.AdvertisePort = conf.BindPort

	peers, err := blocks.NewPeers(conf, seedNodes...)
	if err != nil {
		log.Fatalf("failed to start peer service: %s", err.Error())
	}
	log.Printf("Peer service started on %s:%d", conf.BindAddr, conf.BindPort)

	h := api.New(*httpBind, blocks.NewBlockchain(peers))
	if err := h.Start(); err != nil {
		log.Fatalf("failed to start HTTP service: %s", err.Error())
	}
	log.Printf("Http started on %s", *httpBind)

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Println("exiting...")
}

func mustGetFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err.Error())
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err.Error())
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func mustGetNodeID() string {
	return uuid.Must(uuid.NewV4()).String()
}
