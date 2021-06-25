package peer

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	tls "github.com/libp2p/go-libp2p-tls"
	"github.com/multiformats/go-multiaddr"
)

type Peer struct {
	Id          int64
	Host        host.Host
	Writers     map[int64]chan Message
	WritersLock *sync.RWMutex
}

func (p *Peer) GetAddress() ([]multiaddr.Multiaddr, error) {
	id, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ipfs/%s", p.Host.ID().Pretty()))
	if err != nil {
		return nil, err
	}

	addrs := make([]multiaddr.Multiaddr, 0)
	for _, addr := range p.Host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(id))
	}
	return addrs, nil
}

func (p *Peer) Destroy() {
	if err := p.Host.Close(); err != nil {
		log.Printf("Error: %s\n", err.Error())
	}
}

func (p *Peer) HandleStream() {
	p.Host.SetStreamHandler(protocol.ID("/v2/p2p/simulation"), p.handle)
}

func (p *Peer) handle(stream network.Stream) {
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	in := make(chan struct{}, 2)
	out := make(chan struct{}, 2)
	writer := make(chan Message, 1)

	go p.read(rw, out, in, writer)
	go p.write(rw, out, in, writer)

	out <- <-in
	if err := stream.Close(); err != nil {
		log.Printf("Error: %s\n", err.Error())
	}
}

func (p *Peer) read(rw *bufio.ReadWriter, in chan struct{}, out chan struct{}, writer chan Message) {
	defer func() {
		out <- struct{}{}
	}()

	var isFirst bool = true
	for {
		msg := new(Message)
		if _, err := msg.Read(rw); err != nil {
			log.Printf("Error: %s\n", err.Error())
			break
		}
		if isFirst {
			if msg.Kind != "add" {
				break
			}
			if msg.Author == p.Id {
				break
			}

			msg.Peer = p.Id
			p.WritersLock.Lock()
			p.Writers[msg.Author] = writer
			p.WritersLock.Unlock()
			isFirst = false
		}
	}
}

func (p *Peer) write(rw *bufio.ReadWriter, in chan struct{}, out chan struct{}, writer chan Message) {
	defer func() {
		out <- struct{}{}
	}()

	msg := Message{Author: p.Id, Kind: "add"}
	if _, err := msg.Write(rw); err != nil {
		log.Printf("Error: %s\n", err.Error())
		return
	}

	{
	OUT:
		for {
			select {
			case <-in:
				break OUT

			case msg := <-writer:
				if _, err := msg.Write(rw); err != nil {
					log.Printf("Error: %s\n", err.Error())
					break OUT
				}
			}
		}
	}
}

func createHost(ctx context.Context, id int64, port int) (host.Host, error) {
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 1024, rand.New(rand.NewSource(id)))
	if err != nil {
		return nil, err
	}

	identity := libp2p.Identity(priv)
	addrs := libp2p.ListenAddrStrings([]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)}...)
	_tls := libp2p.Security(tls.ID, tls.New)
	_noise := libp2p.Security(noise.ID, noise.New)
	transports := libp2p.DefaultTransports

	opts := []libp2p.Option{identity, addrs, _tls, _noise, transports}
	return libp2p.New(ctx, opts...)
}

func NewPeer(ctx context.Context, id int64, port int) (*Peer, error) {
	host, err := createHost(ctx, id, port)
	if err != nil {
		return nil, err
	}
	p := Peer{
		Id:          id,
		Host:        host,
		Writers:     make(map[int64]chan Message),
		WritersLock: &sync.RWMutex{},
	}

	p.HandleStream()
	return &p, nil
}
