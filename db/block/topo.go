package block

import (
	"container/heap"
	"fmt"
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

// nodeQueue orders ready nodes by ID without shifting the whole frontier.
type nodeQueue []GraphNode

// Len returns the number of ready nodes.
func (q nodeQueue) Len() int { return len(q) }

// Less orders nodes by identifier.
func (q nodeQueue) Less(i, j int) bool { return q[i].ID() < q[j].ID() }

// Swap exchanges two heap entries.
func (q nodeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

// Push appends a ready node before heap adjustment.
func (q *nodeQueue) Push(node any) { *q = append(*q, node.(GraphNode)) }

// Pop removes the last heap entry and releases its reference.
func (q *nodeQueue) Pop() any {
	last := len(*q) - 1
	node := (*q)[last]
	(*q)[last] = nil
	*q = (*q)[:last]
	return node
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
		for _, depID := range g.from[nod.ID()] {
			indegree[depID]++
		}
	}

	// Nodes are already ID-ordered, so this initial queue is a valid min-heap.
	queue := make(nodeQueue, 0, len(nodes))
	for _, nod := range nodes {
		if indegree[nod.ID()] == 0 {
			queue = append(queue, nod)
		}
	}

	sorted := make([]GraphNode, 0, len(nodes))
	for len(queue) != 0 {
		nod := heap.Pop(&queue).(GraphNode)
		sorted = append(sorted, nod)
		for _, depID := range g.from[nod.ID()] {
			indegree[depID]--
			if indegree[depID] == 0 {
				heap.Push(&queue, g.nodes[depID])
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
