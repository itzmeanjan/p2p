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
	Id          uint32
	Addr        string
	Target      map[uint32]string
	Peers       map[net.Conn]*PeerInfo
	Lock        *sync.RWMutex
	Queue       []*Msg
	QueueLock   *sync.RWMutex
	Log         bool
	Logger      *os.File
	PingClient  chan struct{}
	Traffic     map[uint32]*TrafficCost
	TrafficLock *sync.Mutex
}

type TrafficCost struct {
	In  uint64
	Out uint64
}

type PeerInfo struct {
	Id   uint32
	Addr string
	Ping chan *Msg
}

func New(ctx context.Context, id uint32, addr string, target map[uint32]string, keepLog bool) (*Peer, error) {
	peer := Peer{
		Id:          id,
		Addr:        addr,
		Target:      target,
		Peers:       make(map[net.Conn]*PeerInfo),
		Lock:        &sync.RWMutex{},
		Queue:       make([]*Msg, 0),
		QueueLock:   &sync.RWMutex{},
		Log:         keepLog,
		PingClient:  make(chan struct{}, len(target)),
		Traffic:     make(map[uint32]*TrafficCost),
		TrafficLock: &sync.Mutex{},
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

	for range target {
		peer.PingClient <- struct{}{}
	}
	return &peer, nil
}

// Run a go-routine, used for listening to context cancellation signal
// and on reception closes log file handler --- part of graceful shutdown
func (p *Peer) LogHandler(ctx context.Context) {
	<-ctx.Done()
	if p.Log {
		if err := p.Logger.Close(); err != nil {
			log.Printf("Failed to close log handle : %s\n", err.Error())
		}
	}
}

// Invoked when peer has established connection with another peer
func (p *Peer) Add(conn net.Conn, id uint32, notifier chan *Msg) bool {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	if _, ok := p.Target[id]; ok {
		p.Peers[conn] = &PeerInfo{Id: id, Ping: notifier, Addr: p.Target[id]}
		delete(p.Target, id)
		return true
	}
	return false
}

// Invoked by connection handler method when connection is about to be terminated
// for removing peer from this peer's connected set
func (p *Peer) Remove(conn net.Conn) {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	if info, ok := p.Peers[conn]; ok {
		p.Target[info.Id] = info.Addr
		delete(p.Peers, conn)
		p.PingClient <- struct{}{}
	}
}

// See if this message need to be forwarded to any another peers
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

// Enqueues newly received message, also appends to logs file
// when necessary
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

func (p *Peer) UpdateTrafficCost(id uint32, isInwards bool, size uint64) {
	p.TrafficLock.Lock()
	defer p.TrafficLock.Unlock()

	traffic, ok := p.Traffic[id]
	if !ok {
		traffic = &TrafficCost{}
		p.Traffic[id] = traffic
	}

	if isInwards {
		traffic.In += size
	} else {
		traffic.Out += size
	}
}

func (p *Peer) ExportTrafficCost() error {
	fd, err := os.OpenFile(fmt.Sprintf("%d.traffic.txt", p.Id), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	p.TrafficLock.Lock()
	defer p.TrafficLock.Unlock()

	for peer, traffic := range p.Traffic {
		fd.Write([]byte(fmt.Sprintf("%d; %d; %d\n", peer, traffic.In, traffic.Out)))
	}

	return fd.Close()
}

func (p *Peer) Client(ctx context.Context, done chan struct{}) {
	done <- struct{}{}

	for {
	OUT:
		select {
		case <-ctx.Done():
			return

		case <-p.PingClient:
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
		p.Remove(conn)
		if err := conn.Close(); err != nil {
			log.Printf("[%d <-> %d] Failed to close connection : %s\n", p.Id, id, err.Error())
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	health := make(chan struct{}, 2)
	stopChan := make(chan struct{})
	go p.read(ctx, id, rw, health)
	go p.write(ctx, id, rw, health, stopChan, notifier)

	select {
	case <-ctx.Done():
		return
	case <-health:
		stopChan <- struct{}{}
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
			n, err := msg.read(rw)
			if err != nil {
				log.Printf("[%d <-> %d] Failed to read : %s\n", p.Id, id, err.Error())
				return
			}

			p.UpdateTrafficCost(id, true, uint64(n))
			log.Printf("[%d <-> %d] Received\n", p.Id, id)

			p.Enqueue(msg)
			p.Forward(msg)
		}
	}
}

func (p *Peer) write(ctx context.Context, id uint32, rw *bufio.ReadWriter, health chan struct{}, stopChan chan struct{}, notifier chan *Msg) {
	defer func() {
		health <- struct{}{}
	}()

	msg := Msg{Id: p.Id, Hops: []uint32{p.Id}}
	n, err := msg.write(rw)
	if err != nil {
		log.Printf("[%d <-> %d] Failed to write own message : %s\n", p.Id, id, err.Error())
		return
	}

	p.UpdateTrafficCost(id, false, uint64(n))
	log.Printf("[%d <-> %d] Sent own message\n", p.Id, id)

	for {
		select {
		case <-ctx.Done():
			return

		case <-stopChan:
			return

		case msg := <-notifier:
			msg.Hops = append(msg.Hops, p.Id)
			n, err := msg.write(rw)
			if err != nil {
				log.Printf("[%d <-> %d] Failed to write message : %s\n", p.Id, id, err.Error())
				return
			}

			p.UpdateTrafficCost(id, false, uint64(n))
			log.Printf("[%d <-> %d] Forwarded\n", p.Id, id)
		}
	}
}
