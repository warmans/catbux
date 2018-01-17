package main

//based on https://github.com/otoolep/hraftd and http://lhartikk.github.io/jekyll/update/2017/07/14/chapter1.html

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/hashicorp/serf/serf"
	"github.com/satori/go.uuid"
	"github.com/warmans/catbux/pkg/blocks"
	"github.com/warmans/catbux/pkg/server"
)

const (
	DefaultHTTPBind = "localhost:8686"
)

var (
	httpBindAddr          = flag.String("http-bind", DefaultHTTPBind, "Set the HTTP bind address")
	clusterBindAddr       = flag.String("cluster-bind-addr", "127.0.0.1", "Set the cluster bind address (if port omitted one is generated)")
	clusterAdvertisedAddr = flag.String("cluster-advertise-addr", "", "this is the address other nodes can contact this one on. If blank same as bind-addr")
	clusterSeedNodes      = flag.String("cluster-seed-nodes", "", "address of an existing cluster node(s)")
	clusterTransferPort   = flag.Int("cluster-trasfer-port", 0, "whenever a large sync occurs it will use this port instead of the gossip port")
	nodeID                = flag.String("cluster-node-id", "", "Identifier for the node (leaving blank will generate one)")
)

func main() {
	flag.Parse()

	blockchain := blocks.NewBlockchain()

	cluster, transfers := makeCluster(blockchain)
	defer func() {
		cluster.Close()
		transfers.Close()
	}()

	srv := server.New(*httpBindAddr, blockchain, cluster, transfers)
	if err := srv.Start(); err != nil {
		log.Fatal("server failed: " + err.Error())
	}

	log.Println("Ready!")
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Println("exiting...")
}

func makeCluster(blockchain *blocks.Blockchain) (*server.Cluster, *server.TransferManager) {
	if *nodeID == "" {
		*nodeID = mustGetNodeID()
	}

	//sort out all the hosts/ports
	bindHost, bindPort := mustParseAddr(*clusterBindAddr, mustGetFreePort())
	if *clusterAdvertisedAddr == "" {
		*clusterAdvertisedAddr = fmt.Sprintf("%s:%d", bindHost, bindPort)
	}

	advertiseHost, advertisePort := mustParseAddr(*clusterBindAddr, bindPort)

	for {
		if *clusterTransferPort == 0 || *clusterTransferPort == bindPort {
			*clusterTransferPort = mustGetFreePort()
			break
		}
	}

	transfers := server.NewTransferManager(blockchain)
	go func() {
		log.Fatal(transfers.Listen(fmt.Sprintf("%s:%d", bindHost, *clusterTransferPort)))
	}()

	conf := serf.DefaultConfig()
	conf.Init()
	conf.NodeName = *nodeID
	conf.Tags = map[string]string{"transfer.port": fmt.Sprintf("%d", *clusterTransferPort)}
	conf.MemberlistConfig.BindAddr = bindHost
	conf.MemberlistConfig.BindPort = bindPort
	conf.MemberlistConfig.AdvertiseAddr = advertiseHost
	conf.MemberlistConfig.AdvertisePort = advertisePort

	seedNodes := []string{}
	if *clusterSeedNodes != "" {
		seedNodes = strings.Split(*clusterSeedNodes, ",")
	}

	cluster, err := server.NewCluster(conf, seedNodes...)
	if err != nil {
		log.Fatalf("Failed to start cluster service: %s", err.Error())
	}
	log.Printf("Cluster created on: %s:%d", conf.MemberlistConfig.BindAddr, conf.MemberlistConfig.BindPort)

	return cluster, transfers
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

func mustParseAddr(addr string, defaultPort int) (string, int) {
	parts := strings.Split(addr, ":")
	if len(parts) == 1 {
		return addr, 0
	}
	if len(parts) == 2 {
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			panic(fmt.Sprintf("failed to parse port in %s: %s", addr, err.Error()))
		}
		if port == 0 {
			port = defaultPort
		}
		return parts[0], port
	}
	panic("provided address " + addr + " was invalid. should be host:port or just host to generate a random port")
}
