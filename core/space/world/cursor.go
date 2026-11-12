package space_world

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// replayCursorStoreID is the local state store holding the saved replay.
const replayCursorStoreID = "space-world-replay"

// replayCursorKey is the key of the saved replay in its store.
var replayCursorKey = []byte("cursor")

// replayCursorOperations is how many positions the saved replay may fall
// behind the last one, past the stable positions a checkpoint covers, before
// the adapter saves it again. It bounds what a restart replays while no
// checkpoint lands. While checkpoints land, the positions above the installed
// one are the next checkpoint's stable operations and the unstable ones after
// them, so a bound of fewer than MinCheckpointOperations positions would store
// a World between every two checkpoints.
const replayCursorOperations = sobject.MinCheckpointOperations + 32

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

	// Once saves are refused beyond what replay heals, save again only when
	// the refusal is due for another attempt.
	if !a.saveRefusal.Load().Due(a.now()) {
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
// World's pages store. It starts the next epoch of the replay stage, and ends
// the earlier ones once the record lands: until then they hold the pages of
// the World it saves.
//
// When one of the ending epochs stored a page, every live World may name a
// page only that epoch held. The positions before the saved one are covered
// by the floor, which moves up to the saved World as the save starts, so
// replay computes them again. Every other live World gatherLive returns, the
// served Worlds and the pending candidates, is held in the same root move as
// the saved World, so no World the adapter still serves or proposes names a
// page the ended epoch alone held. One save runs at a time; a save due while
// another runs waits for the next position. The driver goroutine calls it.
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

	// Hold the payloads, then encode the positions. A payload the store
	// refuses counts as a refused save, so replay goes on and the save waits
	// until the refusal is due again.
	r := a.replay
	if err := a.resetPayloads(ctx); err != nil {
		if !errors.Is(err, block.ErrMissingChild) {
			return err
		}
		a.saveRefusal.Store(sobject.NoteStoreRefusal(a.saveRefusal.Load(), err, a.now()))
		a.le.WithError(err).Warn("replay cursor not saved")
		return nil
	}
	root, data, err := a.encodeCursor()
	if err != nil {
		return err
	}

	// When an epoch the save ends stored a page, hold the live Worlds that
	// are no position beside the saved World.
	stored := r.stage.stored()
	worlds, candidates := []*block.BlockRef{root}, []*block.BlockRef(nil)
	if stored {
		l := a.gatherLive()
		worlds, candidates = append(worlds, l.served...), l.candidates
	}

	// Keep them live through the store; a run that ended saves nothing more
	// and opens no epoch, since it ends the stage once its background stores
	// ended.
	done, ok := a.startBackground(slices.Concat(worlds, candidates)...)
	if !ok {
		return nil
	}
	epochs, err := r.stage.rotate(ctx)
	if err != nil {
		done()
		return err
	}
	r.saved = len(r.positions)
	if stored {
		r.floor = max(r.floor, r.saved-1)
	}

	// Write the record in the background.
	started = true
	go func() {
		// Write it, then let the next save start.
		defer a.cursorSaving.Store(false)
		ctx, cancel := a.backgroundContext(ctx)
		defer cancel()
		err := a.writeCursor(ctx, worlds, candidates, data)
		if ctx.Err() == nil {
			a.saveRefusal.Store(sobject.NoteStoreRefusal(a.saveRefusal.Load(), err, a.now()))
		}

		// End the saved epochs once the cursor root holds their World, or
		// keep them for the next save, which retries.
		if err == nil {
			endEpochs(epochs)
		} else {
			r.stage.keep(epochs)
		}
		done()
		if err != nil && ctx.Err() == nil {
			a.le.WithError(err).Warn("replay cursor not saved")
		}
	}()
	return nil
}

// Damaged returns the store refusal of the cursor saves or the trims replay
// could not heal: a save or a trim refused twice in a row with a
// block.MissingChildError, until a later one lands. The body driver shows it
// in the Space's health.
func (a *Adapter) Damaged() *sobject.SOStoreDamage {
	if damage := a.saveRefusal.Load().Damage(); damage != nil {
		return damage
	}
	return a.trimRefusal.Load().Damage()
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

// writeCursor adds worlds, the saved World first, to the replay cursor root
// and candidates to the pending root, then writes data as the saved cursor.
func (a *Adapter) writeCursor(ctx context.Context, worlds, candidates []*block.BlockRef, data []byte) error {
	// Hold them beside the earlier Worlds of each history.
	release, err := a.lockRoots(ctx, slices.Concat(worlds, candidates)...)
	if err != nil {
		return err
	}
	err = a.appendRoots(ctx, &a.cursorWorlds, worlds)
	if err == nil {
		err = a.appendRoots(ctx, &a.pendingWorlds, candidates)
	}
	release()
	if err != nil {
		return err
	}

	// Write the record.
	return a.writeLocal(ctx, replayCursorStoreID, replayCursorKey, data)
}

// loadCursor reads the saved cursor whose head World the replay cursor root
// holds into the replay's restored positions. A cursor whose World the root
// does not hold, or whose outcomes do not lead to its head, is ignored.
func (a *Adapter) loadCursor(ctx context.Context) error {
	// Read the saved cursor.
	data, found, err := a.readLocal(ctx, replayCursorStoreID, replayCursorKey)
	if err != nil || !found {
		return err
	}
	cursor := &ReplayCursor{}
	if err := cursor.UnmarshalVT(data); err != nil {
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
	return slices.EqualFunc(a.GetRetainedRoots(), b.GetRetainedRoots(), (*RetainedRoot).EqualVT)
}

// _ is a type assertion
var _ sobject.DamagedBody = (*Adapter)(nil)
