package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "github.com/itzmeanjan/p2p/v1"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	total := 5
	peers := make([]*v1.Peer, 0, total)
	for i := 0; i < total; i++ {
		peer, err := v1.New(ctx, uint8(i), fmt.Sprintf("127.0.0.1:700%d", i),
			map[string]struct{}{
				fmt.Sprintf("127.0.0.1:%d", (i-1)%total): {},
				fmt.Sprintf("127.0.0.1:%d", (i+1)%total): {},
			}, true)
		if err != nil {
			log.Printf("Failed to start peer %d : %s\n", i, err.Error())
			return
		}
		peers = append(peers, peer)
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
