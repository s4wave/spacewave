//go:build space_adapter_decl

package space_world

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// spacePosition is one operation applied above the installed checkpoint and
// the World after it.
type spacePosition struct {
	// hash identifies the operation.
	hash []byte
	// reason is why replay refused the operation, or empty when it applied.
	reason string
	// world is the World root after the operation, with its transform.
	world *bucket.ObjectRef
	// seqno is the World seqno at world.
	seqno uint64
	// payloads are the roots of the operation's payload outside the World
	// tree, which replay reads again while the operation is above the
	// checkpoint.
	payloads []*block.BlockRef
}

// spaceReplay computes the Worlds of the positions above the installed
// checkpoint on a replay engine: a db/world/block Engine with no Authority
// over the adapter's PendingStore, bound to the World before the position it
// applies. It keeps the World after every position, so a reorder resumes
// after the longest prefix the new order shares, and saves a cursor so a
// restart does the same. The driver's goroutine owns it.
type spaceReplay struct {
	// base is the World of the installed checkpoint.
	base *bucket.ObjectRef
	// height is the height of the installed checkpoint.
	height uint64
	// positions are the applied operations above the checkpoint, in order.
	positions []spacePosition
	// saved is how many positions the saved cursor records.
	saved int
	// engine is the replay engine, bound to bound, or nil.
	engine *world_block.Engine
	// bound is the World engine was built at or last committed.
	bound *bucket.ObjectRef
	// restored are the positions of the saved cursor not yet applied again,
	// which a later Apply of the same operation at the same place adopts.
	restored []spacePosition
}

// before returns the World before the position at index.
func (r *spaceReplay) before(index int) *bucket.ObjectRef {
	if index == 0 {
		return r.base
	}
	return r.positions[index-1].world
}

// run applies op to the World before index on the replay engine and returns
// the position after it.
func (r *spaceReplay) run(ctx context.Context, index int, op *sobject.OrderedOperation) (spacePosition, error) {
	return spacePosition{}, errAdapterUnbuilt
}

// saveCursor moves the replay cursor root onto the World after the last
// position, then writes the cursor record to the SharedObject's local state.
func (r *spaceReplay) saveCursor(ctx context.Context, a *Adapter) error {
	return errAdapterUnbuilt
}

// loadCursor reads the saved cursor whose World the replay cursor root holds
// into restored.
func (r *spaceReplay) loadCursor(ctx context.Context, a *Adapter) error {
	return errAdapterUnbuilt
}
