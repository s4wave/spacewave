package block

import (
	"fmt"
	"slices"
	"strings"
)

// Unorderable is an error listing cyclic components of a block graph.
type Unorderable [][]GraphNode

// Error implements the error interface.
func (e Unorderable) Error() string {
	n := len(e)
	if n > maxCyclicComponentsInError {
		return fmt.Sprintf("block: no topological ordering: %d nodes in %d cyclic components", n, len(e))
	}
	var b strings.Builder
	b.WriteString("block: no topological ordering: cyclic components:")
	for _, component := range e {
		fmt.Fprintf(&b, "\n%v", component)
	}
	return b.String()
}

const maxCyclicComponentsInError = 10

// insertNodeByID inserts nod into the ID-ordered queue.
func insertNodeByID(queue []GraphNode, nod GraphNode) []GraphNode {
	lo, hi := 0, len(queue)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if queue[mid].ID() < nod.ID() {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return slices.Insert(queue, lo, nod)
}

// SortBlockGraph returns a topological ordering of the block graph where
// every edge goes from earlier to later nodes, breaking ties by node ID.
// The caller processes the result in reverse to encode referenced blocks
// before their parents.
func SortBlockGraph(g *BlockGraph) ([]GraphNode, error) {
	nodes := g.Nodes()

	indegree := make(map[int64]int, len(nodes))
	for _, nod := range nodes {
		indegree[nod.ID()] += 0
		for _, dep := range g.From(nod.ID()) {
			indegree[dep.ID()]++
		}
	}

	// queue holds zero-indegree nodes kept ordered by ID for determinism.
	queue := make([]GraphNode, 0, len(nodes))
	for _, nod := range nodes {
		if indegree[nod.ID()] == 0 {
			queue = append(queue, nod)
		}
	}

	sorted := make([]GraphNode, 0, len(nodes))
	for len(queue) != 0 {
		nod := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nod)
		for _, dep := range g.From(nod.ID()) {
			indegree[dep.ID()]--
			if indegree[dep.ID()] == 0 {
				queue = insertNodeByID(queue, dep)
			}
		}
	}

	if len(sorted) != len(nodes) {
		inSort := make(map[int64]bool, len(sorted))
		for _, nod := range sorted {
			inSort[nod.ID()] = true
		}
		remaining := make([]GraphNode, 0, len(nodes)-len(sorted))
		for _, nod := range nodes {
			if !inSort[nod.ID()] {
				remaining = append(remaining, nod)
			}
		}
		return nil, Unorderable{remaining}
	}
	return sorted, nil
}
