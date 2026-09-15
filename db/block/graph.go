package block

import (
	"cmp"
	"slices"
)

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
// Stored references are acyclic; SortBlockGraph also detects cycles introduced
// while constructing an unpublished graph. The owner serializes all access.
type BlockGraph struct {
	// nodes holds the current value for each identifier.
	nodes map[int64]GraphNode
	// edges indexes each directed edge once; adjacency lists contain only IDs.
	edges map[[2]int64]GraphEdge
	// from and to avoid a separate hash table for each usually small adjacency.
	from map[int64][]int64
	to   map[int64][]int64
	// nextID is the next candidate identifier for NewNode.
	nextID int64
}

// NewBlockGraph constructs a new empty block graph.
func NewBlockGraph() *BlockGraph {
	return &BlockGraph{
		nodes: make(map[int64]GraphNode),
		edges: make(map[[2]int64]GraphEdge),
		from:  make(map[int64][]int64),
		to:    make(map[int64][]int64),
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
		return cmp.Compare(a.ID(), b.ID())
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
	for _, toID := range edges {
		if node := g.nodes[toID]; node != nil {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b GraphNode) int {
		return cmp.Compare(a.ID(), b.ID())
	})
	return nodes
}

// Edges returns all edges ordered by from then to node ID.
func (g *BlockGraph) Edges() []GraphEdge {
	edges := make([]GraphEdge, 0, len(g.edges))
	for _, edge := range g.edges {
		edges = append(edges, edge)
	}
	slices.SortFunc(edges, func(a, b GraphEdge) int {
		if a.From().ID() != b.From().ID() {
			return cmp.Compare(a.From().ID(), b.From().ID())
		}
		return cmp.Compare(a.To().ID(), b.To().ID())
	})
	return edges
}

// RemoveEdge removes the edge between two nodes, if present.
func (g *BlockGraph) RemoveEdge(fid, tid int64) {
	key := [2]int64{fid, tid}
	if _, ok := g.edges[key]; !ok {
		return
	}
	delete(g.edges, key)
	removeAdjacent(g.from, fid, tid)
	removeAdjacent(g.to, tid, fid)
}

// removeAdjacent removes one ID without retaining an empty adjacency list.
func removeAdjacent(adjacent map[int64][]int64, id, target int64) {
	ids := adjacent[id]
	if index := slices.Index(ids, target); index >= 0 {
		ids[index] = ids[len(ids)-1]
		ids = ids[:len(ids)-1]
	}
	if len(ids) == 0 {
		delete(adjacent, id)
	} else {
		adjacent[id] = ids
	}
}

// RemoveNode removes a node and its connected edges, if present.
func (g *BlockGraph) RemoveNode(id int64) {
	if g.nodes[id] == nil {
		return
	}
	delete(g.nodes, id)
	for _, toID := range g.from[id] {
		delete(g.edges, [2]int64{id, toID})
		removeAdjacent(g.to, toID, id)
	}
	delete(g.from, id)
	for _, fromID := range g.to[id] {
		delete(g.edges, [2]int64{fromID, id})
		removeAdjacent(g.from, fromID, id)
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
	fid, tid := from.ID(), to.ID()
	key := [2]int64{fid, tid}
	if _, exists := g.edges[key]; !exists {
		g.from[fid] = append(g.from[fid], tid)
		g.to[tid] = append(g.to[tid], fid)
	}
	g.edges[key] = e
}

// graphNode is a bare node identified only by its ID.
type graphNode int64

// ID returns the node identifier.
func (n graphNode) ID() int64 {
	return int64(n)
}
