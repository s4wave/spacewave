package block_gc

import (
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// WalkResult is what a streaming audit walk found.
type WalkResult struct {
	// Counts is what the whole run counted, across restarts.
	Counts *WalkCounts
	// Started is when the run began.
	Started time.Time
	// Resumes counts the times a process took up the run from scratch.
	Resumes uint32
	// Restarted reports that this process found the run older than a day in
	// scratch, and began it again.
	Restarted bool
	// LongestView is this process's longest view of the Volume in each
	// phase.
	LongestView map[WalkPhase]time.Duration
	// UndercountSample names up to sampleSize of the undercounted blocks this
	// process found.
	UndercountSample []*block.BlockRef
	// UnconfirmedSample names up to sampleSize of the unconfirmed candidates
	// this process found.
	UnconfirmedSample []UnconfirmedHole
}
