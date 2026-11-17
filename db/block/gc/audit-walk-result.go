package block_gc

import (
	"github.com/s4wave/spacewave/db/block"
)

// WalkResult is what a streaming audit walk found.
type WalkResult struct {
	// Counts is what the whole run counted, across restarts.
	Counts *WalkCounts
	// UndercountSample names up to sampleSize of the undercounted blocks this
	// process found.
	UndercountSample []*block.BlockRef
	// UnconfirmedSample names up to sampleSize of the unconfirmed candidates
	// this process found.
	UnconfirmedSample []UnconfirmedHole
}
