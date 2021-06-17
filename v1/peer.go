package v1

import (
	"context"
	"log"
	"net"
	"sync"
)

type Peer struct {
	Id     uint8
	Addr   string
	Target map[string]bool
	Peers  map[net.Conn]struct{}
	Lock   *sync.RWMutex
}

func (p *Peer) Add(conn net.Conn) bool {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	addr := conn.RemoteAddr().String()
	if _, ok := p.Target[addr]; ok {
		p.Peers[conn] = struct{}{}
		delete(p.Target, addr)
		return true
	}
	return false
}

func (p *Peer) Server(ctx context.Context, done chan struct{}) {
	done <- struct{}{}

	lis, err := net.Listen("tcp", p.Addr)
	if err != nil {
		log.Printf("Failed to start listening : %s\n", err.Error())
		return
	}
	defer func() {
		if err := lis.Close(); err != nil {
			log.Printf("Failed to close listener : %s\n", err.Error())
		}
	}()

	for {
	ROLL:
		select {
		case <-ctx.Done():
			return

		default:
			conn, err := lis.Accept()
			if err != nil {
				log.Printf("Listener failed : %s\n", err.Error())
				return
			}
			if !p.Add(conn) {
				if err := conn.Close(); err != nil {
					log.Printf("Connection already established with peer : %s\n", err.Error())
					break ROLL
				}
			}

		}
	}

}
