package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/itzmeanjan/p2p/v2/peer"
)

func main() {
	var peerCount int64 = 8
	ctx, cancel := context.WithCancel(context.Background())

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

	for _, p := range peers {
		addrs, err := p.GetAddress()
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		for _, addr := range addrs {
			log.Printf("%d => %s\n", p.Id, addr)
		}
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, syscall.SIGTERM, syscall.SIGINT)

	<-interruptChan
	cancel()
	<-time.After(time.Second)
	for _, p := range peers {
		p.Destroy()
	}
	log.Println("Graceful shutdown !")
}
