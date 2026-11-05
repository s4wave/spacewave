//go:build space_adapter_decl

package space_world

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// spaceTurn is one local writer's turn on a Space World. It holds the
// adapter's turn mutex from Begin to Release, and its base is the writer head
// when it began.
type spaceTurn struct {
	// a is the adapter.
	a *Adapter
	// base is the writer head the turn builds on.
	base world.Head
	// release unlocks the adapter's turn mutex once.
	release func()
}

// Base returns the writer head when the turn began.
func (t *spaceTurn) Base() world.Head {
	return t.base
}

// Admit keeps the candidate's entries in the adapter's PendingStore, records
// it as pending, appends its operation to the queue and makes its root the
// writer head. It returns before the operation is queued, with an Admission
// that waits for the driver to place it.
func (t *spaceTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	return nil, errAdapterUnbuilt
}

// Release unlocks the turn, so the next writer begins on this turn's
// candidate.
func (t *spaceTurn) Release() {}

// _ is a type assertion
var _ world.Turn = (*spaceTurn)(nil)
