package block_gc

import (
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// WalkResult is what a streaming audit walk found.
type WalkResult struct {
	// Counts is what the whole run counted, across restarts.
	Counts *WalkCounts
	// Adoption is the Volume's adoption when the walk ended. Outside Adopted
	// its counts are not in force, and every listed block counts Unadopted.
	Adoption Adoption
	// Started is when the run began.
	Started time.Time
	// Resumes counts the times a process took up the run from scratch.
	Resumes uint32
	// Restart is why this process began the run again instead of resuming
	// the one in scratch, if it did.
	Restart WalkRestart
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

// WalkRestart is why a walk began its run again instead of resuming it.
type WalkRestart int

const (
	// WalkRestartNone is a run begun on empty scratch, or resumed.
	WalkRestartNone WalkRestart = iota
	// WalkRestartAged is a run older than a day, begun again.
	WalkRestartAged
	// WalkRestartUnreadable is a run whose progress record did not
	// unmarshal, such as one an older build wrote; the walk discarded the
	// scratch and began again.
	WalkRestartUnreadable
)

// String names the restart.
func (r WalkRestart) String() string {
	switch r {
	case WalkRestartAged:
		return "older than a day"
	case WalkRestartUnreadable:
		return "progress unreadable, scratch discarded"
	default:
		return "no"
	}
}
