package block_gc

import "time"

// Stats holds GC cycle statistics.
type Stats struct {
	// AtomicSweepCount is the number of candidates atomic sweeps rechecked.
	AtomicSweepCount int
	// AtomicSweepDuration is time spent in atomic sweeps, including rechecks.
	AtomicSweepDuration time.Duration
	// NodesSwept is the number of nodes swept.
	NodesSwept int
	// UnreferencedNodeCount is the number of unreferenced node entries read.
	UnreferencedNodeCount int
	// RemoveNodeRefsCount is the number of nodes whose outgoing refs were removed.
	RemoveNodeRefsCount int
	// RemoveUnreferencedEdgeCount is the number of unreferenced marker edges removed.
	RemoveUnreferencedEdgeCount int
	// OnSweptCount is the number of onSwept callbacks run.
	OnSweptCount int
	// RemoveBlockCount is the number of physical block deletes attempted.
	RemoveBlockCount int
	// Duration is how long the GC cycle took.
	Duration time.Duration
	// UnreferencedScanDuration is time spent listing unreferenced nodes.
	UnreferencedScanDuration time.Duration
	// RemoveNodeRefsDuration is time spent removing outgoing node refs.
	RemoveNodeRefsDuration time.Duration
	// RemoveUnreferencedEdgeDuration is time spent removing unreferenced markers.
	RemoveUnreferencedEdgeDuration time.Duration
	// OnSweptDuration is time spent in onSwept callbacks.
	OnSweptDuration time.Duration
	// RemoveBlockDuration is time spent physically deleting block-backed nodes.
	RemoveBlockDuration time.Duration
}
