package main

import (
	"container/ring"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "github.com/itzmeanjan/p2p/v1"
)

type Info struct {
	Id   uint8
	Addr string
}

func generatePeers(total int) *ring.Ring {
	r := ring.New(total)
	for i := 0; i < r.Len(); i++ {
		r.Value = &Info{
			Id:   uint8(i),
			Addr: fmt.Sprintf("127.0.0.1:700%d", i),
		}
		r = r.Next()
	}
	return r
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	total := 5
	peers := make([]*v1.Peer, 0, total)
	r := generatePeers(total)
	for i := 0; i < total; i++ {
		prv := r.Prev().Value.(*Info)
		nxt := r.Next().Value.(*Info)
		peer, err := v1.New(ctx, uint8(i), fmt.Sprintf("127.0.0.1:700%d", i),
			map[uint8]string{
				prv.Id: prv.Addr,
				nxt.Id: nxt.Addr,
			}, false)
		if err != nil {
			log.Printf("Failed to start peer %d : %s\n", i, err.Error())
			return
		}
		peers = append(peers, peer)
		r = r.Next()
	}

	done := make(chan struct{}, total)
	for _, peer := range peers {
		go peer.Client(ctx, done)
	}

	expected := 0
	for range done {
		expected++
		if expected >= total {
			break
		}
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, syscall.SIGTERM, syscall.SIGINT)

	<-interruptChan
	cancel()
	<-time.After(time.Second)
	log.Println("Shutdown")
}
