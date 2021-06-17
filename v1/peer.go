package v1

import (
	"bufio"
	"context"
	"encoding/binary"
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
	Target    map[uint8]string
	Peers     map[net.Conn]*PeerInfo
	Lock      *sync.RWMutex
	Queue     []*Msg
	QueueLock *sync.RWMutex
	Log       bool
	Logger    io.Writer
}

type PeerInfo struct {
	Id   uint8
	Ping chan *Msg
}

func New(ctx context.Context, id uint8, addr string, target map[uint8]string, keepLog bool) (*Peer, error) {
	peer := Peer{
		Id:        id,
		Addr:      addr,
		Target:    target,
		Peers:     make(map[net.Conn]*PeerInfo),
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

func (p *Peer) Add(conn net.Conn, id uint8, notifier chan *Msg) bool {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	if _, ok := p.Target[id]; ok {
		p.Peers[conn] = &PeerInfo{Id: id, Ping: notifier}
		delete(p.Target, id)
		return true
	}
	return false
}

func (p *Peer) Forward(msg *Msg) {
	for _, info := range p.Peers {
		forward := false
		for _, hop := range msg.Hops {
			if info.Id != hop {
				forward = true
				break
			}
		}

		if forward {
			info.Ping <- msg
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
			var (
				id   uint8
				addr string
			)
			for key, val := range p.Target {
				id = key
				addr = val
			}
			p.Lock.RUnlock()

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Printf("Failed to connect : %s\n", err.Error())
				break OUT
			}

			// Writing peer id to stream
			if err := binary.Write(conn, binary.BigEndian, uint8(p.Id)); err != nil {
				log.Printf("Failed to send id : %s\n", err.Error())
				break OUT
			}

			notifier := make(chan *Msg)
			if !p.Add(conn, id, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("Failed to tear down : %s\n", err.Error())
				}
				log.Printf("[%d] Connection already established : %d\n", p.Id, id)
				break OUT
			}

			log.Printf("[%d] Connected to : %d\n", p.Id, id)
			go p.handleConnection(ctx, id, conn, notifier)
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

			var id uint8
			if err := binary.Read(conn, binary.BigEndian, &id); err != nil {
				log.Printf("Failed to read id : %s\n", err.Error())

				if err := conn.Close(); err != nil {
					log.Printf("Failed to tear down : %s\n", err.Error())
				}
				break OUT
			}
			notifier := make(chan *Msg)
			if !p.Add(conn, id, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("Failed to tear down : %s\n", err.Error())
				}
				log.Printf("[%d] Connection already established : %d\n", p.Id, id)
				break OUT
			}

			log.Printf("[%d] Accepted connection : %d\n", p.Id, id)
			go p.handleConnection(ctx, id, conn, notifier)
		}
	}
}

func (p *Peer) handleConnection(ctx context.Context, id uint8, conn net.Conn, notifier chan *Msg) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Failed to close connection : %s\n", err.Error())
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	health := make(chan struct{}, 2)
	go p.read(ctx, id, rw, health)
	go p.write(ctx, id, rw, health, notifier)

	select {
	case <-ctx.Done():
		return
	case <-health:
		return
	}
}

func (p *Peer) read(ctx context.Context, id uint8, rw *bufio.ReadWriter, health chan struct{}) {
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
				log.Printf("[%d] Failed to read : %s [%d]\n", p.Id, err.Error(), id)
				return
			}

			log.Printf("[%d] Received message : %d\n", p.Id, id)
			p.Forward(msg)
			p.Enqueue(msg)
		}
	}
}

func (p *Peer) write(ctx context.Context, id uint8, rw *bufio.ReadWriter, health chan struct{}, notifier chan *Msg) {
	defer func() {
		health <- struct{}{}
	}()

	msg := Msg{Id: p.Id}
	if _, err := msg.write(rw); err != nil {
		log.Printf("[%d] Failed to write own message : %s [%d]\n", p.Id, err.Error(), id)
		return
	}
	log.Printf("[%d] Sent own message : %d\n", p.Id, id)

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-notifier:
			msg.Hops = append(msg.Hops, id)
			if _, err := msg.write(rw); err != nil {
				log.Printf("[%d] Failed to write message : %s [%d]\n", p.Id, err.Error(), id)
				return
			}
			log.Printf("[%d] Forwarded message : %d\n", p.Id, id)
		}
	}
}
