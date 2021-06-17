package v1

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

type Peer struct {
	Id        uint8
	Addr      string
	Target    map[string]struct{}
	Peers     map[net.Conn]chan *Msg
	Lock      *sync.RWMutex
	Queue     []*Msg
	QueueLock *sync.RWMutex
	Log       bool
	Logger    io.Writer
}

func New(ctx context.Context, id uint8, addr string, target map[string]struct{}, keepLog bool) (*Peer, error) {
	peer := Peer{
		Id:        id,
		Addr:      addr,
		Target:    target,
		Peers:     make(map[net.Conn]chan *Msg),
		Lock:      &sync.RWMutex{},
		Queue:     make([]*Msg, 0),
		QueueLock: &sync.RWMutex{},
		Log:       keepLog,
	}

	if keepLog {
		fd, err := os.OpenFile(fmt.Sprintf("%d.txt", peer.Id), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		peer.Logger = fd
	}
	done := make(chan struct{})
	go peer.Server(ctx, done)
	<-done

	return &peer, nil
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

func (p *Peer) Enqueue(msg *Msg) {
	p.QueueLock.Lock()
	defer p.QueueLock.Unlock()

	p.Queue = append(p.Queue, msg)
	if p.Log {
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Failed to serialise : %s\n", err.Error())
			return
		}
		p.Logger.Write([]byte(fmt.Sprintf("%s\n", data)))
	}
}

func (p *Peer) Client(ctx context.Context, done chan struct{}) {
	done <- struct{}{}

	for {
	OUT:
		select {
		case <-ctx.Done():
			return

		default:
			p.Lock.RLock()
			if len(p.Target) == 0 {
				return
			}
			var addr string
			for target := range p.Target {
				addr = target
			}
			p.Lock.RUnlock()

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				break OUT
			}
			notifier := make(chan *Msg)
			if !p.Add(conn, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("Connection already established with peer : %s\n", err.Error())
				}
				break OUT
			}

			go p.handleConnection(ctx, conn, notifier)
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
	OUT:
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
				}
				break OUT
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
			p.Enqueue(msg)
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
