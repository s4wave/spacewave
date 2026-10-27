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
	// Candidates is the number of unreferenced nodes the last pass listed.
	Candidates int
	// More reports that a budgeted collection stopped with candidates left.
	More bool
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
	// BlockPasses is the number of block passes that marked the graph.
	BlockPasses int
	// MinorPasses is the number of minor passes that marked the young blocks.
	MinorPasses int
	// YoungBlocks is the number of young blocks the last minor pass marked.
	YoungBlocks int
	// MinorStopped says why a minor pass stopped minor passes until a block
	// pass promotes in full, or is empty.
	MinorStopped string
	// MarkDuration is time spent marking blocks.
	MarkDuration time.Duration
	// MarkScans is the number of child list scans the last mark made, zero
	// for a walk.
	MarkScans int
	// MarkLongestRead is the last mark's longest read transaction, zero for
	// a walk.
	MarkLongestRead time.Duration
	// MarkRoots is the number of registered roots the last mark seeded.
	MarkRoots int
	// MarkOwners counts the last mark's owners by kind.
	MarkOwners map[string]OwnerCount
	// SweepLongestTx is the longest sweep transaction, from the start of its
	// work under the writer through its commit.
	SweepLongestTx time.Duration
	// SweepLongestRescue is the longest rescue inside a sweep transaction.
	SweepLongestRescue time.Duration
	// SweepLongestTxRescue is the rescue's share of SweepLongestTx; the rest
	// swept blocks and committed.
	SweepLongestTxRescue time.Duration
	// LiveNodes is the number of nodes the last mark reached.
	LiveNodes int
	// UnreachedBlocks is the number of tracked blocks the last mark did not
	// reach.
	UnreachedBlocks int
	// UnknownBlocks is the number of reached blocks of unknown references the
	// last mark found. A pass that finds any sweeps no block.
	UnknownBlocks int
	// UnknownSample lists some of those blocks.
	UnknownSample []string
	// UnknownNamers lists the node that made each sampled block live, empty
	// for a walk.
	UnknownNamers []string
	// AbsentBlocks is the number of reached blocks without a child list that
	// sweeps treated as leaves because the store does not hold them.
	AbsentBlocks int
	// RescuedNodes is the number of nodes sweeps rescued from the adopt log.
	RescuedNodes int
	// BlocksSwept is the number of blocks block passes removed.
	BlocksSwept int
}

// RecordMark records a block pass's mark.
func (s *Stats) RecordMark(mark *BlockMark) {
	s.BlockPasses++
	s.recordMark(mark)
}

// RecordYoungMark records a minor pass's mark.
func (s *Stats) RecordYoungMark(mark *BlockMark) {
	s.MinorPasses++
	s.YoungBlocks = mark.YoungBlocks
	s.recordMark(mark)
}

// recordMark copies a mark's totals.
func (s *Stats) recordMark(mark *BlockMark) {
	// Copy the reach, the unknown blocks, and the mark's costs.
	s.LiveNodes = mark.Live.Len()
	s.UnreachedBlocks = len(mark.Unreached)
	s.UnknownBlocks = mark.Unknown
	s.UnknownSample, s.UnknownNamers = mark.UnknownSample, mark.UnknownNamers
	s.MarkScans = mark.Scans
	s.MarkLongestRead = mark.LongestRead
	s.MarkRoots, s.MarkOwners = mark.Roots, mark.Owners
}
