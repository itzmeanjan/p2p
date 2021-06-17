package v1

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type Peer struct {
	Id        uint32
	Addr      string
	Target    map[uint32]string
	Peers     map[net.Conn]*PeerInfo
	Lock      *sync.RWMutex
	Queue     []*Msg
	QueueLock *sync.RWMutex
	Log       bool
	Logger    *os.File
}

type PeerInfo struct {
	Id   uint32
	Ping chan *Msg
}

func New(ctx context.Context, id uint32, addr string, target map[uint32]string, keepLog bool) (*Peer, error) {
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
		go peer.LogHandler(ctx)
	}
	done := make(chan struct{})
	go peer.Server(ctx, done)
	<-done

	return &peer, nil
}

func (p *Peer) LogHandler(ctx context.Context) {
	<-ctx.Done()
	if p.Log {
		if err := p.Logger.Close(); err != nil {
			log.Printf("Failed to close log handle : %s\n", err.Error())
		}
	}
}

func (p *Peer) Add(conn net.Conn, id uint32, notifier chan *Msg) bool {
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
	if msg.Id == p.Id {
		return
	}

	for _, info := range p.Peers {
		forward := true
		for _, hop := range msg.Hops {
			if info.Id == hop {
				forward = false
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
				id   uint32
				addr string
			)
			for key, val := range p.Target {
				id = key
				addr = val
			}
			p.Lock.RUnlock()

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Printf("[%d <-> %d] Failed to connect : %s\n", p.Id, id, err.Error())
				break OUT
			}

			// Writing peer id to stream
			if err := binary.Write(conn, binary.BigEndian, p.Id); err != nil {
				log.Printf("[%d <-> %d] Failed to send id : %s\n", p.Id, id, err.Error())
				break OUT
			}

			notifier := make(chan *Msg, 1)
			if !p.Add(conn, id, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("[%d <-> %d] Failed to tear down : %s\n", p.Id, id, err.Error())
				}
				log.Printf("[%d <-> %d] Connection already established\n", p.Id, id)
				break OUT
			}

			log.Printf("[%d <-> %d] Connected\n", p.Id, id)
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

			var id uint32
			if err := binary.Read(conn, binary.BigEndian, &id); err != nil {
				log.Printf("[%d] Failed to read id : %s\n", p.Id, err.Error())

				if err := conn.Close(); err != nil {
					log.Printf("[%d] Failed to tear down : %s\n", p.Id, err.Error())
				}
				break OUT
			}

			notifier := make(chan *Msg, 1)
			if !p.Add(conn, id, notifier) {
				if err := conn.Close(); err != nil {
					log.Printf("[%d <-> %d] Failed to tear down : %s\n", p.Id, id, err.Error())
				}
				log.Printf("[%d <-> %d] Connection already established\n", p.Id, id)
				break OUT
			}

			log.Printf("[%d <-> %d] Accepted connection\n", p.Id, id)
			go p.handleConnection(ctx, id, conn, notifier)
		}
	}
}

func (p *Peer) handleConnection(ctx context.Context, id uint32, conn net.Conn, notifier chan *Msg) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("[%d <-> %d] Failed to close connection : %s\n", p.Id, id, err.Error())
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

func (p *Peer) read(ctx context.Context, id uint32, rw *bufio.ReadWriter, health chan struct{}) {
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
				log.Printf("[%d <-> %d] Failed to read : %s\n", p.Id, id, err.Error())
				return
			}

			log.Printf("[%d <-> %d] Received %v\n", p.Id, id, msg)
			p.Enqueue(msg)
			p.Forward(msg)
		}
	}
}

func (p *Peer) write(ctx context.Context, id uint32, rw *bufio.ReadWriter, health chan struct{}, notifier chan *Msg) {
	defer func() {
		health <- struct{}{}
	}()

	msg := Msg{Id: p.Id, Hops: []uint32{p.Id}}
	if _, err := msg.write(rw); err != nil {
		log.Printf("[%d <-> %d] Failed to write own message : %s\n", p.Id, id, err.Error())
		return
	}
	log.Printf("[%d <-> %d] Sent own message\n", p.Id, id)

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-notifier:
			msg.Hops = append(msg.Hops, p.Id)
			if _, err := msg.write(rw); err != nil {
				log.Printf("[%d <-> %d] Failed to write message : %s\n", p.Id, id, err.Error())
				return
			}
			log.Printf("[%d <-> %d] Forwarded\n", p.Id, id)
		}
	}
}
