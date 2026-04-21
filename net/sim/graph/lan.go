package graph

// LAN connects together machines and networks with implied many-to-many links.
// This simulates a wireless network, for example.
type LAN struct {
	Node
}

// AddLAN adds a lan to the network graph.
func AddLAN(g *Graph) *LAN {
	l := &LAN{Node: g.BuildNode()}
	g.AddNode(l)
	return l
}

// AddPeer adds a peer to the LAN.
func (l *LAN) AddPeer(g *Graph, p *Peer) Edge {
	e := g.BuildEdge(l, p)
	g.AddEdge(e)
	return e
}

// AddConnectionToLAN adds a connection to another lan.
func (l *LAN) AddConnectionToLAN(g *Graph, ol *LAN) Edge {
	e := g.BuildEdge(l, ol)
	g.AddEdge(e)
	return e
}

// GetAssociatedPeers returns all peers directly linked to the lan.
func (l *LAN) GetAssociatedPeers(g *Graph) []*Peer {
	var peers []*Peer
	nodes := g.From(l)
	for nodes.Next() {
		n := nodes.Node()
		np, isPeer := n.(*Peer)
		if isPeer {
			peers = append(peers, np)
		}
	}
	return peers
}
