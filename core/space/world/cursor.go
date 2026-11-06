package space_world

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// replayCursorStoreID is the local state store holding the saved replay.
const replayCursorStoreID = "space-world-replay"

// replayCursorKey is the key of the saved replay in its store.
var replayCursorKey = []byte("cursor")

// replayCursorOperations is how many positions the saved replay may fall
// behind the last one before the adapter saves it again, which bounds what a
// restart replays while no checkpoint lands.
const replayCursorOperations = 32

// maybeSaveCursor saves the replay cursor when it fell replayCursorOperations
// behind the last position, or when the kept pages near their bound. A save
// over the bound also drops from memory the pages of every World but the
// last, which replay computes again from the checkpoint if it needs them. The
// driver goroutine calls it.
func (a *Adapter) maybeSaveCursor(ctx context.Context) error {
	// Wait for enough positions or pages; a transient replay saves nothing.
	r := a.replay
	full := a.pages.Full()
	if a.opts.transient {
		return nil
	}
	if len(r.positions) == 0 || (!full && len(r.positions)-r.saved < replayCursorOperations) {
		return nil
	}

	// Wait while the last position is below the floor: its World left memory
	// unstored, so the cursor root cannot hold it.
	if len(r.positions) <= r.floor {
		return nil
	}
	if err := a.saveCursor(ctx); err != nil {
		return err
	}

	// Drop the earlier Worlds' pages over the bound.
	if full {
		r.floor = len(r.positions) - 1
		a.sweep()
	}
	return nil
}

// saveCursor holds the payloads above the checkpoint and encodes the replay
// cursor, then, beside the driver's apply loop, adds the World after the last
// position to the replay cursor root and writes the cursor record, so the
// record never names an unheld World and the driver keeps applying while the
// World's pages store. One save runs at a time; a save due while another
// runs waits for the next position. The driver goroutine calls it.
func (a *Adapter) saveCursor(ctx context.Context) error {
	// Skip while a save runs.
	if !a.cursorSaving.CompareAndSwap(false, true) {
		return nil
	}
	started := false
	defer func() {
		if !started {
			a.cursorSaving.Store(false)
		}
	}()

	// Hold the payloads, then encode the positions.
	r := a.replay
	if err := a.resetPayloads(ctx); err != nil {
		return err
	}
	root, data, err := a.encodeCursor()
	if err != nil {
		return err
	}
	r.saved = len(r.positions)

	// Hold the head World and write the record in the background; a run that
	// ended saves nothing more.
	done, ok := a.startBackground(root)
	if !ok {
		return nil
	}
	started = true
	go func() {
		// Write it, then let the next save start.
		defer a.cursorSaving.Store(false)
		ctx, cancel := a.backgroundContext(ctx)
		defer cancel()
		err := a.writeCursor(ctx, root, data)
		done()

		// Report a failure the next save retries.
		if err != nil && ctx.Err() == nil {
			a.le.WithError(err).Warn("replay cursor not saved")
		}
	}()
	return nil
}

// encodeCursor returns the World after the last position and the encoded
// replay cursor that ends there. The driver goroutine calls it.
func (a *Adapter) encodeCursor() (*block.BlockRef, []byte, error) {
	// Record each position's outcome after the base.
	r := a.replay
	head, _ := r.head()
	cursor := &ReplayCursor{Base: r.base, Head: head, Height: r.height}
	prev := r.base
	for _, pos := range r.positions {
		outcome := &ReplayCursorOutcome{
			Hash:     pos.op.Hash,
			Reason:   pos.reason,
			World:    pos.state.GetHeadRef().GetRootRef(),
			Payloads: pos.payloads,
			Seqno:    pos.seqno,
		}
		if !sameRetained(prev, pos.state) {
			outcome.RetainedRoots = &RetainedRootSet{Roots: pos.state.GetRetainedRoots()}
		}
		cursor.Outcomes = append(cursor.Outcomes, outcome)
		prev = pos.state
	}

	// Encode it.
	data, err := cursor.MarshalVT()
	if err != nil {
		return nil, nil, err
	}
	return head.GetHeadRef().GetRootRef().Clone(), data, nil
}

// writeCursor adds root to the replay cursor root, then writes data as the
// saved cursor.
func (a *Adapter) writeCursor(ctx context.Context, root *block.BlockRef, data []byte) error {
	// Hold the head World beside the earlier cursor Worlds.
	release, err := a.lockRoots(ctx, root)
	if err != nil {
		return err
	}
	err = a.addHistory(ctx, &a.cursorWorlds, root)
	release()
	if err != nil {
		return err
	}

	// Write the record.
	store, releaseStore, err := a.so.AccessLocalStateStore(ctx, replayCursorStoreID, nil)
	if err != nil {
		return err
	}
	defer releaseStore()
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	return kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, replayCursorKey, data)
	})
}

// loadCursor reads the saved cursor whose head World the replay cursor root
// holds into the replay's restored positions. A cursor whose World the root
// does not hold, or whose outcomes do not lead to its head, is ignored.
func (a *Adapter) loadCursor(ctx context.Context) error {
	// Read the saved cursor.
	store, release, err := a.so.AccessLocalStateStore(ctx, replayCursorStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, false) }
	cursor := &ReplayCursor{}
	var found bool
	err = kvtx.RunTransaction(ctx, false, open, func(ctx context.Context, tx kvtx.Tx) error {
		data, ok, err := tx.Get(ctx, replayCursorKey)
		if err != nil || !ok {
			return err
		}
		found = true
		return cursor.UnmarshalVT(data)
	})
	if err != nil || !found {
		return err
	}

	// Require the cursor root to hold its head.
	head := cursor.GetHead()
	if !a.cursorWorlds.holds(head.GetHeadRef().GetRootRef()) {
		return nil
	}

	// Restore the World after each outcome.
	positions := make([]spacePosition, len(cursor.GetOutcomes()))
	state := cursor.GetBase()
	for i, outcome := range cursor.GetOutcomes() {
		state = restoredState(state, head, outcome)
		positions[i] = spacePosition{
			op:       &sobject.OrderedOperation{Hash: outcome.GetHash()},
			reason:   outcome.GetReason(),
			state:    state,
			seqno:    outcome.GetSeqno(),
			payloads: outcome.GetPayloads(),
		}
	}
	if !state.EqualVT(head) {
		a.le.Warn("saved replay does not lead to its head, replaying from the checkpoint")
		return nil
	}
	a.replay.restored = positions
	a.replay.restoredFrom = cursor.GetBase()
	return nil
}

// restoredState returns the World after the saved outcome of the operation
// that followed prev. A World after the base differs from head only in its
// root and retained roots, which the outcome records.
func restoredState(prev, head *InnerState, outcome *ReplayCursorOutcome) *InnerState {
	// Name a changed root as head names its own.
	ref := prev.GetHeadRef()
	initializes := ref.GetEmpty() && outcome.GetReason() == "" && !head.GetHeadRef().GetEmpty()
	if initializes || !refEqual(outcome.GetWorld(), ref.GetRootRef()) {
		ref = head.GetHeadRef().CloneVT()
		ref.RootRef = outcome.GetWorld()
	}

	// Keep an unchanged World.
	roots := prev.GetRetainedRoots()
	if set := outcome.GetRetainedRoots(); set != nil {
		roots = set.GetRoots()
	} else if ref == prev.GetHeadRef() {
		return prev
	}
	return &InnerState{HeadRef: ref, RetainedRoots: roots}
}

// sameRetained reports whether a and b retain the same roots.
func sameRetained(a, b *InnerState) bool {
	ar, br := a.GetRetainedRoots(), b.GetRetainedRoots()
	if len(ar) != len(br) {
		return false
	}
	for i := range ar {
		if !ar[i].EqualVT(br[i]) {
			return false
		}
	}
	return true
}
