package block

import "slices"

// GraphNode is a node in the in-memory block graph.
type GraphNode interface {
	// ID returns the node identifier.
	ID() int64
}

// GraphEdge is a directed edge between block graph nodes.
type GraphEdge interface {
	// From returns the referencing node.
	From() GraphNode
	// To returns the referenced node.
	To() GraphNode
}

// AttributedNode is a graph node carrying a DOT identity and render
// attributes for graph visualizations.
type AttributedNode interface {
	GraphNode
	// DOTID returns the DOT node identifier.
	DOTID() string
	// Attributes returns the graph attributes.
	Attributes() []BlockGraphAttribute
}

// BlockGraph is the in-memory graph of blocks and their references built
// while a transaction traverses and mutates block state. The write path
// orders work with SortBlockGraph; visualizations read Nodes and Edges.
//
// A BlockGraph is a directed acyclic graph by construction: edges are
// content-hashed references, so a cycle cannot be expressed.
type BlockGraph struct {
	nodes  map[int64]GraphNode
	from   map[int64]map[int64]GraphEdge
	to     map[int64]map[int64]GraphEdge
	nextID int64
}

// NewBlockGraph constructs a new empty block graph.
func NewBlockGraph() *BlockGraph {
	return &BlockGraph{
		nodes:  make(map[int64]GraphNode),
		from:   make(map[int64]map[int64]GraphEdge),
		to:     make(map[int64]map[int64]GraphEdge),
		nextID: 0,
	}
}

// NewNode allocates a fresh node identifier wrapped in a bare node value.
// The caller typically stores the result in a handle before the handle is
// added with AddNode.
func (g *BlockGraph) NewNode() GraphNode {
	for {
		id := g.nextID
		g.nextID++
		if _, exists := g.nodes[id]; !exists {
			return graphNode(id)
		}
	}
}

// AddNode adds a node to the graph. Panics if the node ID already exists.
func (g *BlockGraph) AddNode(n GraphNode) {
	if _, exists := g.nodes[n.ID()]; exists {
		panic("block: node id collision")
	}
	g.nodes[n.ID()] = n
	if n.ID() >= g.nextID {
		g.nextID = n.ID() + 1
	}
}

// Node looks up a node by ID, returning nil if absent.
func (g *BlockGraph) Node(id int64) GraphNode {
	return g.nodes[id]
}

// Nodes returns all nodes ordered by ID.
func (g *BlockGraph) Nodes() []GraphNode {
	nodes := make([]GraphNode, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b GraphNode) int {
		return int(a.ID() - b.ID())
	})
	return nodes
}

// From returns the nodes directly referenced by id, ordered by ID.
func (g *BlockGraph) From(id int64) []GraphNode {
	edges := g.from[id]
	if len(edges) == 0 {
		return nil
	}
	nodes := make([]GraphNode, 0, len(edges))
	for toID := range edges {
		if node := g.nodes[toID]; node != nil {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b GraphNode) int {
		return int(a.ID() - b.ID())
	})
	return nodes
}

// Edges returns all edges ordered by from then to node ID.
func (g *BlockGraph) Edges() []GraphEdge {
	edges := make([]GraphEdge, 0)
	for _, byTo := range g.from {
		for _, edge := range byTo {
			edges = append(edges, edge)
		}
	}
	slices.SortFunc(edges, func(a, b GraphEdge) int {
		if a.From().ID() != b.From().ID() {
			return int(a.From().ID() - b.From().ID())
		}
		return int(a.To().ID() - b.To().ID())
	})
	return edges
}

// RemoveEdge removes the edge between two nodes, if present.
func (g *BlockGraph) RemoveEdge(fid, tid int64) {
	if g.from[fid] != nil {
		delete(g.from[fid], tid)
		if len(g.from[fid]) == 0 {
			delete(g.from, fid)
		}
	}
	if g.to[tid] != nil {
		delete(g.to[tid], fid)
		if len(g.to[tid]) == 0 {
			delete(g.to, tid)
		}
	}
}

// RemoveNode removes a node and its connected edges, if present.
func (g *BlockGraph) RemoveNode(id int64) {
	if g.nodes[id] == nil {
		return
	}
	delete(g.nodes, id)
	for toID := range g.from[id] {
		if g.to[toID] != nil {
			delete(g.to[toID], id)
			if len(g.to[toID]) == 0 {
				delete(g.to, toID)
			}
		}
	}
	delete(g.from, id)
	for fromID := range g.to[id] {
		if g.from[fromID] != nil {
			delete(g.from[fromID], id)
			if len(g.from[fromID]) == 0 {
				delete(g.from, fromID)
			}
		}
	}
	delete(g.to, id)
}

// SetEdge adds or replaces an edge, adding its endpoints as needed.
// Panics on self-edges.
func (g *BlockGraph) SetEdge(e GraphEdge) {
	from := e.From()
	to := e.To()
	if from.ID() == to.ID() {
		panic("block: adding self edge")
	}
	if g.nodes[from.ID()] == nil {
		g.AddNode(from)
	}
	g.nodes[from.ID()] = from
	if g.nodes[to.ID()] == nil {
		g.AddNode(to)
	}
	g.nodes[to.ID()] = to
	if g.from[from.ID()] == nil {
		g.from[from.ID()] = make(map[int64]GraphEdge)
	}
	g.from[from.ID()][to.ID()] = e
	if g.to[to.ID()] == nil {
		g.to[to.ID()] = make(map[int64]GraphEdge)
	}
	g.to[to.ID()][from.ID()] = e
}

// graphNode is a bare node identified only by its ID.
type graphNode int64

// ID returns the node identifier.
func (n graphNode) ID() int64 {
	return int64(n)
}
