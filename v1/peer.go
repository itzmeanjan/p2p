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
	Peers  map[net.Conn]chan *Msg
	Lock   *sync.RWMutex
}

func (p *Peer) Add(conn net.Conn, notifier chan *Msg) bool {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	addr := conn.RemoteAddr().String()
	if _, ok := p.Target[addr]; ok {
		p.Peers[conn] = notifier
		delete(p.Target, addr)
		return true
	}
	return false
}

func (p *Peer) Forward(msg *Msg) {
	for peer, ping := range p.Peers {
		addr := peer.RemoteAddr().String()
		forward := false
		for _, hop := range msg.Hops {
			if addr != hop {
				forward = true
				break
			}
		}

		if forward {
			ping <- msg
		}
	}
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
			notifier := make(chan *Msg)
			if !p.Add(conn, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("Connection already established with peer : %s\n", err.Error())
					break ROLL
				}
			}

			go p.handleConnection(ctx, conn, notifier)
		}
	}
}

func (p *Peer) handleConnection(ctx context.Context, conn net.Conn, notifier chan *Msg) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Failed to close connection : %s\n", err.Error())
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	health := make(chan struct{}, 2)
	go p.read(ctx, rw, health)
	go p.write(ctx, rw, health, notifier)

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

			p.Forward(msg)
		}
	}
}

func (p *Peer) write(ctx context.Context, rw *bufio.ReadWriter, health chan struct{}, notifier chan *Msg) {
	defer func() {
		health <- struct{}{}
	}()

	msg := Msg{Id: p.Id, Author: p.Addr}
	if _, err := msg.write(rw); err != nil {
		log.Printf("Failed to write own message : %s\n", err.Error())
		return
	}
	if err := rw.Flush(); err != nil {
		log.Printf("Failed to flush : %s\n", err.Error())
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-notifier:
			msg.Hops = append(msg.Hops, p.Addr)
			if _, err := msg.write(rw); err != nil {
				log.Printf("Failed to write message : %s\n", err.Error())
				return
			}
			if err := rw.Flush(); err != nil {
				log.Printf("Failed to flush : %s\n", err.Error())
				return
			}
		}
	}
}
