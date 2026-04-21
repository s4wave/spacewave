package graph

import (
	"context"
	"maps"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// FactoryCtor is a constructor for a factory.
type FactoryCtor func(bus.Bus) controller.Factory

// FactoryAdder is a adder for a factory.
type FactoryAdder func(b bus.Bus, sr *static.Resolver)

// Peer is a participating peer in the network.
// This corresponds to a controller bus and an inproc transport.
type Peer struct {
	Node
	// peerPriv is the private key
	peerPriv crypto.PrivKey
	// peerID is the peer id
	peerID peer.ID
	// extraControllers contains extra controller configurations.
	extraControllers configset.ConfigSet
	// extraFactories contains extra factory constructors.
	extraFactories []FactoryCtor
	// extraFactoryAdders contains extra factory adders.
	extraFactoryAdders []FactoryAdder
}

// GenerateAddPeer generates a peer private key and adds it.
func GenerateAddPeer(ctx context.Context, g *Graph) (*Peer, error) {
	npeer, err := peer.NewPeer(nil)
	if err != nil {
		return nil, err
	}
	peerPriv, err := npeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	return AddPeer(g, peerPriv)
}

// AddPeer adds a peer to the network graph.
func AddPeer(g *Graph, peerPriv crypto.PrivKey) (*Peer, error) {
	peerID, err := peer.IDFromPrivateKey(peerPriv)
	if err != nil {
		return nil, err
	}
	l := &Peer{
		Node:             g.BuildNode(),
		peerID:           peerID,
		peerPriv:         peerPriv,
		extraControllers: configset.ConfigSet{},
	}
	g.AddNode(l)
	return l, nil
}

// AddFactory adds a factory constructor to the set.
func (p *Peer) AddFactory(ct FactoryCtor) {
	p.extraFactories = append(p.extraFactories, ct)
}

// AddFactoryAdder adds a factory adder to the set.
func (p *Peer) AddFactoryAdder(ct FactoryAdder) {
	p.extraFactoryAdders = append(p.extraFactoryAdders, ct)
}

// GetExtraFactories returns the slice of extra factories.
func (p *Peer) GetExtraFactories() []FactoryCtor {
	return p.extraFactories
}

// GetExtraFactoryAdders returns the slice of extra factory adders.
func (p *Peer) GetExtraFactoryAdders() []FactoryAdder {
	return p.extraFactoryAdders
}

// MergeConfigSet merges in a configset to the extra controllers set.
func (p *Peer) MergeConfigSet(other configset.ConfigSet) {
	maps.Copy(p.extraControllers, other)
}

// AddConfig adds a controller configuration to the peer.
func (p *Peer) AddConfig(id string, conf config.Config) {
	p.AddControllerConfig(id, configset.NewControllerConfig(1, conf))
}

// GetConfigSet returns the extra controllers config set.
func (p *Peer) GetConfigSet() configset.ConfigSet {
	return p.extraControllers
}

// DeleteConfig removes a configuration from the configset with id.
func (p *Peer) DeleteConfig(id string) {
	delete(p.extraControllers, id)
}

// AddControllerConfig adds a controller configuration to the peer.
func (p *Peer) AddControllerConfig(id string, conf configset.ControllerConfig) {
	p.extraControllers[id] = conf
}

// GetPeerID returns the peer ID.
func (p *Peer) GetPeerID() peer.ID {
	return p.peerID
}

// GetPeerPriv returns the peer private key.
func (p *Peer) GetPeerPriv() crypto.PrivKey {
	return p.peerPriv
}

// GetLinkedPeers returns all peers that should have a link with the peer.
// This includes other peers directly linked as well as those linked by a lan or multiple lans.
func (p *Peer) GetLinkedPeers(g *Graph) []*Peer {
	var stack []Node = g.FromNodes(p) //nolint:staticcheck
	var peers []*Peer
	seenNodes := map[Node]struct{}{p: {}}
	for len(stack) != 0 {
		nn := stack[len(stack)-1]
		stack[len(stack)-1] = nil
		stack = stack[:len(stack)-1]
		if _, ok := seenNodes[nn]; ok {
			continue
		}
		seenNodes[nn] = struct{}{}
		switch nnp := nn.(type) {
		case *LAN:
			// if it's a lan, continue traversal
			stack = append(stack, g.FromNodes(nn)...)
		case *Peer:
			peers = append(peers, nnp)
		}
	}
	return peers
}

// AllPeers returns the set of all peers in the graph.
func (g *Graph) AllPeers() []*Peer {
	it := g.graph.Nodes()
	var nodes []*Peer
	for it.Next() {
		nod := it.Node()
		p, pOk := nod.(*Peer)
		if pOk {
			nodes = append(nodes, p)
		}
	}
	return nodes
}
