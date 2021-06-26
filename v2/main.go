package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/itzmeanjan/p2p/v2/peer"
	"github.com/multiformats/go-multiaddr"
)

func connect(ctx context.Context, p *peer.Peer, self int, peers []*peer.Peer, count int) error {
	if count > len(peers)-1 {
		return fmt.Errorf("can't make %d neighbours with %d peers", count, len(peers))
	}

	log.Printf("[%d] Finding neighbours\n", p.Id)
	neighbours := make(map[multiaddr.Multiaddr]struct{})
	for len(neighbours) != count {
		n := rand.Intn(len(peers))
		if n == self {
			continue
		}
		p := peers[n]
		addrs, err := p.GetAddress()
		if err != nil {
			return err
		}
		neighbours[addrs[0]] = struct{}{}
	}

	for n := range neighbours {
		if err := p.Connect(ctx, n); err != nil {
			return err
		}
		<-time.After(time.Second)
	}

	return nil
}

func main() {
	var peerCount int64 = 8
	var neighbourCount int = 2
	ctx, cancel := context.WithCancel(context.Background())

	// Creating libp2p peers
	peers := make([]*peer.Peer, 0, peerCount)
	var i int64
	for ; i < peerCount; i++ {
		p, err := peer.NewPeer(ctx, i+1, 7001+int(i))
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		peers = append(peers, p)
	}

	peer.InitContext(ctx)
	// Displaying peer listen addresses
	for i := 0; i < int(peerCount); i++ {
		p := peers[i]
		addrs, err := p.GetAddress()
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		for _, addr := range addrs {
			log.Printf("%d => %s\n", p.Id, addr)
		}
	}

	<-time.After(time.Second)
	// Neighbour finding phase
	for i := 0; i < int(peerCount); i++ {
		if err := connect(ctx, peers[i], i, peers, neighbourCount); err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		<-time.After(time.Second)
	}

	<-time.After(4 * time.Second)
	// Network topology probing phase
	for i := 0; i < int(peerCount); i++ {
		p := peers[i]
		p.Probe()
		<-time.After(2 * time.Second)
	}

	<-time.After(4 * time.Second)
	log.Printf("Waiting for exit signal !")

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, syscall.SIGTERM, syscall.SIGINT)

	<-interruptChan
	cancel()
	<-time.After(time.Second)
	// Destroy p2p nodes; export traffic & network structure
	// data into log files
	for i := 0; i < int(peerCount); i++ {
		p := peers[i]
		p.Destroy()
		if err := p.ExportNetwork(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
		if err := p.ExportTraffic(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
	}
	log.Println("Graceful shutdown !")
}
