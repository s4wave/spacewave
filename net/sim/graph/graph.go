package graph

import "slices"

// Node is the base type for a node in the graph.
type Node interface {
	// ID returns the node identifier.
	ID() int64
}

// Edge is the base type for an edge in the graph.
type Edge interface {
	// From returns one endpoint of the edge.
	From() Node
	// To returns the other endpoint of the edge.
	To() Node
}

// Graph is an instance of a graph of connected nodes.
type Graph struct {
	nodes  map[int64]Node
	adj    map[int64]map[int64]Edge
	nextID int64
}

// NewGraph constructs a new network graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:  make(map[int64]Node),
		adj:    make(map[int64]map[int64]Edge),
		nextID: 0,
	}
}

// node is a bare node identified only by its ID.
type node int64

// ID returns the node identifier.
func (n node) ID() int64 {
	return int64(n)
}

// simpleEdge is an edge between two nodes.
type simpleEdge struct {
	from Node
	to   Node
}

// From returns one endpoint of the edge.
func (e simpleEdge) From() Node {
	return e.from
}

// To returns the other endpoint of the edge.
func (e simpleEdge) To() Node {
	return e.to
}

// BuildNode constructs a new base node. The node is not added to the graph
// until AddNode or AddEdge references it.
func (g *Graph) BuildNode() Node {
	for {
		id := g.nextID
		g.nextID++
		if _, exists := g.nodes[id]; !exists {
			return node(id)
		}
	}
}

// AddNode adds a node to the network graph. Panics on duplicate node IDs.
func (g *Graph) AddNode(n Node) {
	if _, exists := g.nodes[n.ID()]; exists {
		panic("sim: node id collision")
	}
	g.nodes[n.ID()] = n
	if n.ID() >= g.nextID {
		g.nextID = n.ID() + 1
	}
}

// BuildEdge constructs a new edge between two nodes.
func (g *Graph) BuildEdge(from, to Node) Edge {
	return simpleEdge{from: from, to: to}
}

// AddEdge adds an edge to the network graph, adding its endpoints as
// needed. Panics on self-edges.
func (g *Graph) AddEdge(edge Edge) {
	from, to := edge.From(), edge.To()
	if from.ID() == to.ID() {
		panic("sim: adding self edge")
	}
	if _, exists := g.nodes[from.ID()]; !exists {
		g.AddNode(from)
	}
	if _, exists := g.nodes[to.ID()]; !exists {
		g.AddNode(to)
	}
	if g.adj[from.ID()] == nil {
		g.adj[from.ID()] = make(map[int64]Edge)
	}
	g.adj[from.ID()][to.ID()] = edge
	if g.adj[to.ID()] == nil {
		g.adj[to.ID()] = make(map[int64]Edge)
	}
	g.adj[to.ID()][from.ID()] = edge
}

// From returns all nodes directly connected to the node, ordered by ID.
func (g *Graph) From(n Node) []Node {
	neighbors := g.adj[n.ID()]
	if len(neighbors) == 0 {
		return nil
	}
	nodes := make([]Node, 0, len(neighbors))
	for id := range neighbors {
		nodes = append(nodes, g.nodes[id])
	}
	sortNodes(nodes)
	return nodes
}

// FromNodes returns From as a []Node.
func (g *Graph) FromNodes(n Node) []Node {
	return g.From(n)
}

// ShortestPath finds the shortest path between the two nodes by hop
// count. Returns a 0 len slice if not found.
func (g *Graph) ShortestPath(n1, n2 Node) []Node {
	if n1.ID() == n2.ID() {
		return []Node{n1}
	}
	prev := map[int64]Node{n1.ID(): nil}
	queue := []Node{n1}
	for len(queue) != 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range g.From(cur) {
			if _, visited := prev[next.ID()]; visited {
				continue
			}
			prev[next.ID()] = cur
			if next.ID() == n2.ID() {
				// walk the chain back to the start
				path := []Node{next}
				for at := prev[next.ID()]; at != nil; at = prev[at.ID()] {
					path = append(path, at)
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path
			}
			queue = append(queue, next)
		}
	}
	return nil
}

// AllNodes returns the set of all nodes in the graph, ordered by ID.
func (g *Graph) AllNodes() []Node {
	nodes := make([]Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	sortNodes(nodes)
	return nodes
}

// AllEdges returns the set of all edges in the graph, ordered by endpoint
// IDs.
func (g *Graph) AllEdges() []Edge {
	edges := make([]Edge, 0)
	for _, neighbors := range g.adj {
		for _, e := range neighbors {
			edges = append(edges, e)
		}
	}
	slices.SortFunc(edges, func(a, b Edge) int {
		if a.From().ID() != b.From().ID() {
			return int(a.From().ID() - b.From().ID())
		}
		return int(a.To().ID() - b.To().ID())
	})
	return edges
}

// sortNodes orders nodes by ID.
func sortNodes(nodes []Node) {
	slices.SortFunc(nodes, func(a, b Node) int { return int(a.ID() - b.ID()) })
}
