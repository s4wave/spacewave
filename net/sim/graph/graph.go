package graph

import (
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/traverse"
)

// Node is the base type for a node in the graph.
type Node = graph.Node

// Edge is the base type for an edge in the graph.
type Edge = graph.Edge

// Graph is an instance of a graph of connected nodes.
type Graph struct {
	// graph is the inner graph
	graph *simple.UndirectedGraph
}

// NewGraph constructs a new network graph.
func NewGraph() *Graph {
	return &Graph{
		graph: simple.NewUndirectedGraph(),
	}
}

// BuildNode constructs a new base node.
func (g *Graph) BuildNode() Node {
	return g.graph.NewNode()
}

// AddNode adds a node to the network graph.
func (g *Graph) AddNode(node Node) {
	g.graph.AddNode(node)
}

// BuildEdge constructs a new edge between two nodes.
func (g *Graph) BuildEdge(from, to graph.Node) graph.Edge {
	return g.graph.NewEdge(from, to)
}

// AddEdge adds an edge to the network graph.
func (g *Graph) AddEdge(edge Edge) {
	g.graph.SetEdge(edge)
	g.graph.Edges()
}

// From returns all nodes that can be reached directly from the node.
func (g *Graph) From(node Node) graph.Nodes {
	return g.graph.From(node.ID())
}

// FromNodes returns From as a []Node.
func (g *Graph) FromNodes(node Node) []Node {
	gn := g.From(node)
	var nodes []Node
	for gn.Next() {
		nodes = append(nodes, gn.Node())
	}
	return nodes
}

// Subgraph returns all nodes that are in the subgraph containing the node.
func (g *Graph) Subgraph(node Node) []graph.Node {
	var nodes []graph.Node
	df := &traverse.DepthFirst{
		Visit: func(gn graph.Node) {
			nodes = append(nodes, gn)
		},
	}
	_ = df.Walk(g.graph, node, nil)
	return nodes
}

// ShortestPath finds the shortest path between the two nodes.
// Returns 0 len slice if not found.
func (g *Graph) ShortestPath(n1, n2 Node) []Node {
	sp := path.DijkstraAllPaths(g.graph)
	p, _, _ := sp.Between(n1.ID(), n2.ID())
	return p
}

// AllNodes returns the set of all nodes in the graph.
func (g *Graph) AllNodes() []Node {
	it := g.graph.Nodes()
	var nodes []Node
	for it.Next() {
		nodes = append(nodes, it.Node())
	}
	return nodes
}

// AllEdges returns the set of all edges in the graph.
func (g *Graph) AllEdges() []Edge {
	it := g.graph.Edges()
	var edges []Edge
	for it.Next() {
		edges = append(edges, it.Edge())
	}
	return edges
}
