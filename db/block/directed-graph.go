package block

import (
	"slices"

	"gonum.org/v1/gonum/graph"
)

type directedGraph struct {
	nodes map[int64]graph.Node
	from  map[int64]map[int64]graph.Edge
	to    map[int64]map[int64]graph.Edge

	nextID int64
}

func newDirectedGraph() *directedGraph {
	return &directedGraph{
		nodes: make(map[int64]graph.Node),
		from:  make(map[int64]map[int64]graph.Edge),
		to:    make(map[int64]map[int64]graph.Edge),
	}
}

func (g *directedGraph) AddNode(n graph.Node) {
	if _, exists := g.nodes[n.ID()]; exists {
		panic("block: node id collision")
	}
	g.nodes[n.ID()] = n
	if n.ID() >= g.nextID {
		g.nextID = n.ID() + 1
	}
}

func (g *directedGraph) Edge(uid, vid int64) graph.Edge {
	if g.from[uid] == nil {
		return nil
	}
	return g.from[uid][vid]
}

func (g *directedGraph) Edges() graph.Edges {
	edges := make([]graph.Edge, 0)
	for _, byTo := range g.from {
		for _, edge := range byTo {
			edges = append(edges, edge)
		}
	}
	slices.SortFunc(edges, func(a, b graph.Edge) int {
		if a.From().ID() != b.From().ID() {
			return int(a.From().ID() - b.From().ID())
		}
		return int(a.To().ID() - b.To().ID())
	})
	return &edgeSliceIterator{edges: edges}
}

func (g *directedGraph) From(id int64) graph.Nodes {
	edges := g.from[id]
	if len(edges) == 0 {
		return &nodeSliceIterator{}
	}
	nodes := make([]graph.Node, 0, len(edges))
	for toID := range edges {
		if node := g.nodes[toID]; node != nil {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b graph.Node) int {
		return int(a.ID() - b.ID())
	})
	return &nodeSliceIterator{nodes: nodes}
}

func (g *directedGraph) HasEdgeBetween(xid, yid int64) bool {
	return g.HasEdgeFromTo(xid, yid) || g.HasEdgeFromTo(yid, xid)
}

func (g *directedGraph) HasEdgeFromTo(uid, vid int64) bool {
	if g.from[uid] == nil {
		return false
	}
	_, ok := g.from[uid][vid]
	return ok
}

func (g *directedGraph) NewEdge(from, to graph.Node) graph.Edge {
	return simpleEdge{from: from, to: to}
}

func (g *directedGraph) NewNode() graph.Node {
	for {
		id := g.nextID
		g.nextID++
		if _, exists := g.nodes[id]; !exists {
			return graphNode(id)
		}
	}
}

func (g *directedGraph) Node(id int64) graph.Node {
	return g.nodes[id]
}

func (g *directedGraph) Nodes() graph.Nodes {
	nodes := make([]graph.Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b graph.Node) int {
		return int(a.ID() - b.ID())
	})
	return &nodeSliceIterator{nodes: nodes}
}

func (g *directedGraph) NodeWithID(id int64) (graph.Node, bool) {
	node := g.nodes[id]
	if node != nil {
		return node, false
	}
	return graphNode(id), true
}

func (g *directedGraph) RemoveEdge(fid, tid int64) {
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

func (g *directedGraph) RemoveNode(id int64) {
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

func (g *directedGraph) SetEdge(e graph.Edge) {
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
		g.from[from.ID()] = make(map[int64]graph.Edge)
	}
	g.from[from.ID()][to.ID()] = e
	if g.to[to.ID()] == nil {
		g.to[to.ID()] = make(map[int64]graph.Edge)
	}
	g.to[to.ID()][from.ID()] = e
}

func (g *directedGraph) To(id int64) graph.Nodes {
	edges := g.to[id]
	if len(edges) == 0 {
		return &nodeSliceIterator{}
	}
	nodes := make([]graph.Node, 0, len(edges))
	for fromID := range edges {
		if node := g.nodes[fromID]; node != nil {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b graph.Node) int {
		return int(a.ID() - b.ID())
	})
	return &nodeSliceIterator{nodes: nodes}
}

type graphNode int64

func (n graphNode) ID() int64 {
	return int64(n)
}

type simpleEdge struct {
	from graph.Node
	to   graph.Node
}

func (e simpleEdge) From() graph.Node {
	return e.from
}

func (e simpleEdge) ReversedEdge() graph.Edge {
	return simpleEdge{from: e.to, to: e.from}
}

func (e simpleEdge) To() graph.Node {
	return e.to
}

type nodeSliceIterator struct {
	nodes []graph.Node
	index int
}

func (i *nodeSliceIterator) Len() int {
	return max(len(i.nodes)-i.index, 0)
}

func (i *nodeSliceIterator) Next() bool {
	if i.index >= len(i.nodes) {
		return false
	}
	i.index++
	return true
}

func (i *nodeSliceIterator) Node() graph.Node {
	if i.index == 0 || i.index > len(i.nodes) {
		return nil
	}
	return i.nodes[i.index-1]
}

func (i *nodeSliceIterator) NodeSlice() []graph.Node {
	if i.index >= len(i.nodes) {
		return nil
	}
	nodes := slices.Clone(i.nodes[i.index:])
	i.index = len(i.nodes)
	return nodes
}

func (i *nodeSliceIterator) Reset() {
	i.index = 0
}

type edgeSliceIterator struct {
	edges []graph.Edge
	index int
}

func (i *edgeSliceIterator) Len() int {
	return max(len(i.edges)-i.index, 0)
}

func (i *edgeSliceIterator) Next() bool {
	if i.index >= len(i.edges) {
		return false
	}
	i.index++
	return true
}

func (i *edgeSliceIterator) Edge() graph.Edge {
	if i.index == 0 || i.index > len(i.edges) {
		return nil
	}
	return i.edges[i.index-1]
}

func (i *edgeSliceIterator) EdgeSlice() []graph.Edge {
	if i.index >= len(i.edges) {
		return nil
	}
	edges := slices.Clone(i.edges[i.index:])
	i.index = len(i.edges)
	return edges
}

func (i *edgeSliceIterator) Reset() {
	i.index = 0
}

// _ is a type assertion.
var _ graph.Directed = ((*directedGraph)(nil))

// _ is a type assertion.
var _ graph.NodeAdder = ((*directedGraph)(nil))

// _ is a type assertion.
var _ graph.NodeRemover = ((*directedGraph)(nil))

// _ is a type assertion.
var _ graph.EdgeAdder = ((*directedGraph)(nil))

// _ is a type assertion.
var _ graph.EdgeRemover = ((*directedGraph)(nil))
