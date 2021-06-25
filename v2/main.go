package main

import (
	"container/ring"
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

	peers := ring.New(int(peerCount))
	var i int64
	for ; i < peerCount; i++ {
		p, err := peer.NewPeer(ctx, i+1, 7001+int(i))
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		peers.Value = p
		peers = peers.Next()
	}

	peer.InitContext(ctx)
	peers.Do(func(i interface{}) {
		p := i.(*peer.Peer)
		addrs, err := p.GetAddress()
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		for _, addr := range addrs {
			log.Printf("%d => %s\n", p.Id, addr)
		}
	})

	for i := 0; i < peers.Len(); i++ {
		cur := peers.Value.(*peer.Peer)
		nxt := peers.Next().Value.(*peer.Peer)
		addrs, err := nxt.GetAddress()
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		if err := cur.Connect(ctx, addrs[0]); err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		peers = peers.Next()
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, syscall.SIGTERM, syscall.SIGINT)

	<-interruptChan
	cancel()
	<-time.After(time.Second)
	peers.Do(func(i interface{}) {
		p := i.(*peer.Peer)
		p.Destroy()
	})
	log.Println("Graceful shutdown !")
}
