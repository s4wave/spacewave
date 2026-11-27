package world_block

import (
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// pendingChange binds evidence to a candidate and its successful admission.
// Matching roots alone cannot prove touched names: different failed proposals
// can cancel different edits and produce the same final root and revision.
type pendingChange struct {
	// base is the root this candidate built on.
	base *block.BlockRef
	// root is the result this candidate proposed.
	root *block.BlockRef
	// changes is the candidate's detached touched-name summary.
	changes *changeSummary
	// confirmed reports successful durability at this candidate's exact root.
	confirmed atomic.Bool
}

// confirm permits feed consumers to use a successfully admitted summary.
func (p *pendingChange) confirm() {
	if p != nil {
		p.confirmed.Store(true)
	}
}

// summary returns evidence only after admission confirms the exact transition.
func (p *pendingChange) summary(before, after *block.BlockRef) *changeSummary {
	if p.confirmed.Load() && p.base.EqualsRef(before) && p.root.EqualsRef(after) {
		return p.changes
	}
	return nil
}
