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

func connect(ctx context.Context, p *peer.Peer, self int, peers []*peer.Peer, count int, record map[int64]map[int64]struct{}) error {
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

		if v, ok := record[p.Id]; ok {
			if _, ok := v[_p.Id]; ok {
				continue
			}
		}

		if v, ok := record[p.Id]; ok {
			v[_p.Id] = struct{}{}
		} else {
			v = make(map[int64]struct{})
			v[_p.Id] = struct{}{}
			record[p.Id] = v
		}

		if v, ok := record[_p.Id]; ok {
			v[p.Id] = struct{}{}
		} else {
			v = make(map[int64]struct{})
			v[p.Id] = struct{}{}
			record[_p.Id] = v
		}
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

	rand.Seed(time.Now().UnixNano())

	<-time.After(time.Second)
	// Neighbour finding phase
	record := make(map[int64]map[int64]struct{})
	for i := 0; i < int(peerCount); i++ {
		if err := connect(ctx, peers[i], i, peers, neighbourCount, record); err != nil {
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

	// Export traffic & network structure
	// data into log files
	for i := 0; i < int(peerCount); i++ {
		p := peers[i]
		if err := p.ExportNetwork(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
		if err := p.ExportTraffic(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
	}

	cancel()
	<-time.After(time.Second)

	// Destroy p2p nodes
	for i := 0; i < int(peerCount); i++ {
		peers[i].Destroy()
	}
	log.Println("Graceful shutdown !")
}
