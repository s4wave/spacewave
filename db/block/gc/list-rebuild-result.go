package block_gc

import "github.com/s4wave/spacewave/db/block"

// ListRebuildResult is what a list rebuild verified and wrote.
type ListRebuildResult struct {
	// Verified counts the lists checked against their block's decoded refs.
	Verified uint64
	// Rewritten counts those written because they were missing or lacked
	// children the decoded refs name.
	Rewritten uint64
	// Dropped counts the blocks whose decoded refs lack a child their stored
	// list names. A stored list can only miss children, so every edge it holds
	// is real, and such a report means a wrong constructor or a decoder bug:
	// the rebuild keeps the stored list and leaves the block on the front.
	Dropped uint64
	// DroppedSample names up to sampleSize of them.
	DroppedSample []*block.BlockRef
	// Front counts the blocks on the settle's front when Finish returned:
	// unverified blocks a holder reaches that no Verify wrote. Any keeps the
	// old generation Open, so the collector frees nothing it holds.
	Front uint64
	// FrontSample names up to sampleSize of them.
	FrontSample []*block.BlockRef
}
