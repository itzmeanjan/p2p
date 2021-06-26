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

func connect(ctx context.Context, p *peer.Peer, self int, peers []*peer.Peer, count int, record_1 map[int64]int64, record_2 map[int64]int64) error {
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
		_p := peers[n]
		addrs, err := _p.GetAddress()
		if err != nil {
			return err
		}

		if v, ok := record_1[p.Id]; ok && v == _p.Id {
			continue
		}
		if v, ok := record_2[p.Id]; ok && v == _p.Id {
			continue
		}

		record_1[p.Id] = _p.Id
		record_2[_p.Id] = p.Id
		neighbours[addrs[0]] = struct{}{}
	}

	for n := range neighbours {
		if err := p.Connect(ctx, n); err != nil {
			return err
		}
		<-time.After(10 * time.Millisecond)
	}

	return nil
}

func main() {
	var peerCount int64 = 32
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
	record_1 := make(map[int64]int64)
	record_2 := make(map[int64]int64)
	for i := 0; i < int(peerCount); i++ {
		if err := connect(ctx, peers[i], i, peers, neighbourCount, record_1, record_2); err != nil {
			log.Printf("Error: %s\n", err.Error())
			return
		}
		<-time.After(100 * time.Millisecond)
	}

	// Network topology probing phase
	<-time.After(time.Second)
	peers[0].Probe()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, syscall.SIGTERM, syscall.SIGINT)

	{
	OUT:
		for {
			select {
			case <-interruptChan:
				break OUT
			case <-time.After(time.Second * 4):
				log.Printf("Waiting for exit signal !")
			}
		}
	}

	cancel()
	<-time.After(time.Second)
	// Destroy p2p nodes; export traffic & network structure
	// data into log files
	for i := 0; i < int(peerCount); i++ {
		p := peers[i]
		if err := p.ExportNetwork(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
		if err := p.ExportTraffic(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
		p.Destroy()
	}
	log.Println("Graceful shutdown !")
}
