package v1

import (
	"bufio"
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

func (p *Peer) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Failed to close connection : %s\n", err.Error())
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	health := make(chan struct{})
	go p.read(ctx, rw, health)
	go p.write(ctx, rw, health)

	select {
	case <-ctx.Done():
		return
	case <-health:
		return
	}
}

func (p *Peer) read(ctx context.Context, rw *bufio.ReadWriter, health chan struct{}) {
	defer func() {
		health <- struct{}{}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		default:
			msg := new(Msg)
			if _, err := msg.read(rw); err != nil {
				log.Printf("Failed to read : %s\n", err.Error())
				return
			}
		}
	}
}

func (p *Peer) write(ctx context.Context, rw *bufio.ReadWriter, health chan struct{}) {
	defer func() {
		health <- struct{}{}
	}()

	msg := Msg{Id: p.Id, Hops: []uint8{p.Id}}
	if _, err := msg.write(rw); err != nil {
		log.Printf("Failed to write own message : %s\n", err.Error())
		return
	}
	if err := rw.Flush(); err != nil {
		log.Printf("Failed to flush : %s\n", err.Error())
		return
	}
}
