package peer

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	tls "github.com/libp2p/go-libp2p-tls"
	"github.com/multiformats/go-multiaddr"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
)

var parentCtx context.Context

func InitContext(ctx context.Context) {
	parentCtx = ctx
}

type Peer struct {
	Id          int64
	Host        host.Host
	Writers     map[int64]chan Message
	WritersLock *sync.RWMutex
	Network     *simple.UndirectedGraph
	NetworkLock *sync.RWMutex
}

type node struct {
	simple.Node
	attrs []encoding.Attribute
}

func (n *node) DOTID() string {
	return fmt.Sprintf("Peer %d", n.ID())
}

func (n *node) String() string {
	return n.DOTID()
}

func (n *node) Attributes() []encoding.Attribute {
	return n.attrs
}

type edge struct {
	simple.WeightedEdge
}

func (e *edge) Attributes() []encoding.Attribute {
	return []encoding.Attribute{
		{Key: "weight", Value: fmt.Sprintf("%f", e.W)},
	}
}

func newEdge(frm graph.Node, to graph.Node) *edge {
	return &edge{
		WeightedEdge: simple.WeightedEdge{
			F: frm,
			T: to,
			W: 0.0,
		}}
}

func (p *Peer) UpdateTraffic(peer int64, amount int) {
	p.NetworkLock.Lock()
	defer p.NetworkLock.Unlock()

	e := p.Network.Edge(p.Id, peer)
	if e == nil {
		return
	}
	_e := e.(*edge)
	_e.W += float64(amount)
}

func (p *Peer) ExportTraffic() error {
	p.NetworkLock.RLock()
	defer p.NetworkLock.RUnlock()

	file := fmt.Sprintf("%d.traffic.csv", p.Id)
	fd, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err := fd.Close(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
	}()

	edges := p.Network.Edges()
	for edges.Next() {
		e := edges.Edge().(*edge)
		fd.WriteString(fmt.Sprintf("%d; %d; %f\n", e.F.ID(), e.T.ID(), e.W))
	}

	log.Printf("[%d] Logged network traffic into %s\n", p.Id, file)
	return nil
}

func (p *Peer) InitNetwork(peer int64) {
	p.NetworkLock.Lock()
	defer p.NetworkLock.Unlock()

	node_1 := p.Network.Node(p.Id)
	if node_1 == nil {
		p.Network.AddNode(&node{
			Node: simple.Node(p.Id),
			attrs: []encoding.Attribute{
				{Key: "color", Value: "black"},
				{Key: "style", Value: "filled"},
				{Key: "fontcolor", Value: "white"}}})
	}
	node_2 := p.Network.Node(peer)
	if node_2 == nil {
		p.Network.AddNode(&node{
			Node: simple.Node(peer),
			attrs: []encoding.Attribute{
				{Key: "style", Value: "filled"}}})
	}
	if p.Network.HasEdgeBetween(p.Id, peer) {
		return
	}
	p.Network.SetEdge(newEdge(p.Network.Node(p.Id), p.Network.Node(peer)))
}

func (p *Peer) UpdateNetwork(msg *Message) {
	p.NetworkLock.Lock()
	defer p.NetworkLock.Unlock()

	for i := 0; i < len(msg.Hops); i++ {
		hop := msg.Hops[i]
		n := p.Network.Node(hop)
		if n == nil {
			p.Network.AddNode(&node{
				Node: simple.Node(hop),
				attrs: []encoding.Attribute{
					{Key: "style", Value: "filled"}}})
		}
	}

	for i := 0; i < len(msg.Hops)-1; i++ {
		hop_1 := msg.Hops[i]
		hop_2 := msg.Hops[i+1]
		if !p.Network.HasEdgeBetween(hop_1, hop_2) {
			p.Network.SetEdge(newEdge(p.Network.Node(hop_1), p.Network.Node(hop_2)))
		}
	}
}

func (p *Peer) UpdateNetworkWithNonProbingMessage(msg *Message) {
	p.NetworkLock.Lock()

	if n := p.Network.Node(msg.Author); n == nil {
		p.Network.AddNode(&node{
			Node: simple.Node(msg.Author),
			attrs: []encoding.Attribute{
				{Key: "style", Value: "filled"}}})
	}

	if n := p.Network.Node(msg.Peer); n == nil {
		p.Network.AddNode(&node{
			Node: simple.Node(msg.Peer),
			attrs: []encoding.Attribute{
				{Key: "style", Value: "filled"}}})
	}

	if msg.Kind == "add" {
		if !p.Network.HasEdgeBetween(msg.Author, msg.Peer) {
			p.Network.SetEdge(newEdge(p.Network.Node(msg.Author), p.Network.Node(msg.Peer)))
		}
	} else {
		if p.Network.HasEdgeBetween(msg.Author, msg.Peer) {
			p.Network.RemoveEdge(msg.Author, msg.Peer)
		}
	}

	p.NetworkLock.Unlock()
	p.UpdateNetwork(msg)
}

func (p *Peer) ExportNetwork() error {
	p.NetworkLock.RLock()
	defer p.NetworkLock.RUnlock()

	out, err := dot.Marshal(p.Network, fmt.Sprintf("P2P Network viewed by Peer_%d", p.Id), "", "  ")
	if err != nil {
		return err
	}

	file := fmt.Sprintf("%d_%d.dot", p.Id, time.Now().Unix())
	fd, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err := fd.Close(); err != nil {
			log.Printf("Error: %s\n", err.Error())
		}
	}()
	n, err := fd.Write(out)
	if err != nil {
		return err
	}
	log.Printf("[%d] Logged perceived network into %s [%d bytes]\n", p.Id, file, n)
	return nil
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

func (p *Peer) Probe() {
	log.Printf("[%d] Started probing\n", p.Id)
	msg := Message{Author: p.Id, Kind: "probe", Hops: []int64{}}
	p.broadcast_all(&msg)
}

func (p *Peer) AddToPeerStore(id peer.ID, addrs []multiaddr.Multiaddr) {
	p.Host.Peerstore().AddAddrs(id, addrs, peerstore.PermanentAddrTTL)
}

func (p *Peer) Connect(ctx context.Context, addr multiaddr.Multiaddr) error {
	addInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}
	p.AddToPeerStore(addInfo.ID, addInfo.Addrs)
	stream, err := p.Host.NewStream(ctx, addInfo.ID, protocol.ID("/v2/p2p/simulation"))
	if err != nil {
		return err
	}

	func(stream network.Stream) {
		go p.handle(stream)
	}(stream)
	return nil
}

func (p *Peer) HandleStream() {
	p.Host.SetStreamHandler(protocol.ID("/v2/p2p/simulation"), p.handle)
}

func (p *Peer) handle(stream network.Stream) {
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	in := make(chan struct{}, 2)
	out := make(chan struct{}, 2)
	writer := make(chan Message, 128)
	idChan := make(chan int64, 2)

	go p.read(rw, out, in, writer, idChan)
	go p.write(rw, out, in, writer, idChan)

	id := <-idChan
	out <- <-in

	p.WritersLock.Lock()
	defer p.WritersLock.Unlock()
	delete(p.Writers, id)
	log.Printf("[%d] Disconnected from %d\n", p.Id, id)

	if err := stream.Close(); err != nil {
		log.Printf("Error: %s\n", err.Error())
	}

	msg := Message{Author: p.Id, Kind: "del", Peer: id}
	p.broadcast_all(&msg)
}

func (p *Peer) read(rw *bufio.ReadWriter, in chan struct{}, out chan struct{}, writer chan Message, idChan chan int64) {
	defer func() {
		out <- struct{}{}
	}()

	var isFirst bool = true
	var id int64
	{
	OUT:
		for {
			select {
			case <-parentCtx.Done():
				break OUT

			default:
				msg := new(Message)
				n, err := msg.Read(rw)
				if err != nil {
					log.Printf("Error: %s\n", err.Error())
					break OUT
				}
				if isFirst {
					if msg.Kind != "add" {
						break OUT
					}
					if msg.Author == p.Id {
						break OUT
					}

					msg.Peer = p.Id
					p.WritersLock.Lock()
					p.Writers[msg.Author] = writer
					p.WritersLock.Unlock()
					isFirst = false
					id = msg.Author
					idChan <- id
					idChan <- id
					p.InitNetwork(id)
					log.Printf("[%d] Connected to %d\n", p.Id, id)
				} else {
					log.Printf("[%d] Received from %d: %v\n", p.Id, id, msg)
				}

				if msg.Kind == "probe" {
					p.UpdateNetwork(msg)
				} else {
					p.UpdateNetworkWithNonProbingMessage(msg)
				}
				p.UpdateTraffic(id, n)
				p.broadcast(msg)
			}
		}
	}
}

func (p *Peer) broadcast_all(msg *Message) {
	p.WritersLock.RLock()
	defer p.WritersLock.RUnlock()

	for _, ping := range p.Writers {
		ping <- *msg
	}
}

func (p *Peer) broadcast(msg *Message) {
	if msg.Author == p.Id {
		return
	}

	// Message has already hopped over this peer
	for _, hop := range msg.Hops {
		if hop == p.Id {
			return
		}
	}

	p.WritersLock.RLock()
	defer p.WritersLock.RUnlock()

	// Peer from which message is received i.e.
	// most recent hop
	receivedFrom := msg.Hops[len(msg.Hops)-1]
	for id, ping := range p.Writers {
		if receivedFrom == id {
			continue
		}

		var traversed bool
		for i := 0; i < len(msg.Hops)-1; i++ {
			hop := msg.Hops[i]
			if i == 0 && hop == id {
				break
			}
			if id == hop {
				traversed = true
				break
			}
		}
		if !traversed {
			ping <- *msg
		}
	}
}

func (p *Peer) write(rw *bufio.ReadWriter, in chan struct{}, out chan struct{}, writer chan Message, idChan chan int64) {
	defer func() {
		out <- struct{}{}
	}()

	msg := Message{Author: p.Id, Kind: "add", Hops: []int64{p.Id}}
	mL, err := msg.Write(rw)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return
	}
	var id int64

	defer func() {
		if id != 0 {
			p.UpdateTraffic(id, mL)
		}
	}()

	{
	OUT:
		for {
			select {
			case <-parentCtx.Done():
				break OUT

			case <-in:
				break OUT

			case _id := <-idChan:
				id = _id

			case msg := <-writer:
				msg.Hops = append(msg.Hops, p.Id)
				n, err := msg.Write(rw)
				if err != nil {
					log.Printf("Error: %s\n", err.Error())
					break OUT
				}

				if id != 0 {
					p.UpdateTraffic(id, n)
				}
				log.Printf("[%d] Wrote to %d\n", p.Id, id)
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
		Network:     simple.NewUndirectedGraph(),
		NetworkLock: &sync.RWMutex{},
	}

	p.HandleStream()
	return &p, nil
}
