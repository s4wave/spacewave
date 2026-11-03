package sobject_world_engine

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// backfillStoreID is the local state store holding this device's backfill
// choice for the World.
const backfillStoreID = "world-backfill"

// backfillKey is the key of the backfill choice in its store. It is present
// when backfill is on.
var backfillKey = []byte("enabled")

// backfillProofStoreID is the local state store holding the completion proofs
// of the backfilled Worlds.
const backfillProofStoreID = "accepted-world-retention"

// backfillBackoff spaces the retries of a failed backfill copy, such as one
// whose peers went away.
var backfillBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 1000,
		MaxInterval:     60000,
		Multiplier:      2,
	},
}

// BackfillEngine is a World engine whose device can keep a complete local copy
// of the World. Without backfill the device reads blocks on demand and keeps
// the blocks it wrote or read while the World still references them.
//
// The served engine implements it, not the BusEngine that world.StartEngine
// returns: assert it inside world.AccessEngine.
type BackfillEngine interface {
	// SetBackfill chooses whether this device copies the installed World and
	// the retained roots into its local store in the background. The choice
	// persists on this device.
	SetBackfill(ctx context.Context, enabled bool) error
	// GetBackfill watches this device's backfill choice.
	GetBackfill() ccontainer.Watchable[bool]
	// GetBackfillState watches the outcome of the last finished copy, nil
	// while backfill is off or before its first copy finishes.
	GetBackfillState() ccontainer.Watchable[*BackfillState]
	// RecheckBackfill restarts the copy so it walks every block of the
	// covered roots, trusting no completion proof, and fetches each one the
	// store lacks. A false proof, such as one an older build recorded, hides
	// the blocks under it from every other copy. It returns once the
	// recheck starts; GetBackfillState reports its outcome.
	RecheckBackfill(ctx context.Context) error
	// GetHead returns the installed World root and its seqno, read once the
	// installed World is durable. A copy that covers the same root and seqno
	// holds the same World.
	GetHead(ctx context.Context) (*block.BlockRef, uint64, error)
}

// IsComplete reports whether the local store held every block of the covered
// roots.
func (s *BackfillState) IsComplete() bool {
	return s != nil && s.GetHoles() == 0
}

// SetBackfill stores this device's backfill choice and starts or stops the
// copy.
func (e *soEngine) SetBackfill(ctx context.Context, enabled bool) error {
	// Serialize choices so the stored and the published choice agree.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "set-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()

	// Store the choice, then publish it and start or stop the copy. Stopping
	// cancels the copy before it can publish, so its outcome clears for good.
	if err := writeBackfill(ctx, e.so, enabled); err != nil {
		return err
	}
	e.backfillChoice.SetValue(enabled)
	e.backfill.SetState(enabled)
	if !enabled {
		e.backfillState.SetValue(nil)
	}
	return nil
}

// GetBackfill watches this device's backfill choice.
func (e *soEngine) GetBackfill() ccontainer.Watchable[bool] {
	return e.backfillChoice
}

// GetBackfillState watches the outcome of the last finished copy.
func (e *soEngine) GetBackfillState() ccontainer.Watchable[*BackfillState] {
	return e.backfillState
}

// RecheckBackfill restarts the copy as a recheck.
func (e *soEngine) RecheckBackfill(ctx context.Context) error {
	// Serialize with the choice, so a recheck never runs with backfill off.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "recheck-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	if !e.backfillChoice.GetValue() {
		return errors.New("backfill is off")
	}

	// Clear the last outcome so a waiter sees the recheck's, then restart.
	e.backfillRecheck.Store(true)
	e.backfillState.SetValue(nil)
	e.backfill.RestartRoutine()
	return nil
}

// readBackfill reads this device's backfill choice for the World of so.
func readBackfill(ctx context.Context, so sobject.SharedObject) (bool, error) {
	// Open the choice's store.
	store, release, err := so.AccessLocalStateStore(ctx, backfillStoreID, nil)
	if err != nil {
		return false, err
	}
	defer release()

	// Read the choice.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	_, found, err := tx.Get(ctx, backfillKey)
	return found, err
}

// writeBackfill stores this device's backfill choice for the World of so.
func writeBackfill(ctx context.Context, so sobject.SharedObject, enabled bool) error {
	// Open the choice's store.
	store, release, err := so.AccessLocalStateStore(ctx, backfillStoreID, nil)
	if err != nil {
		return err
	}
	defer release()

	// Set or delete the key.
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if enabled {
			return tx.Set(ctx, backfillKey, []byte{1})
		}
		return tx.Delete(ctx, backfillKey)
	})
}

// executeBackfill copies the installed World and the retained roots into the
// local store, then again after each World change, and publishes the outcome of
// each copy. Completion proofs make each copy skip the subtrees an earlier copy
// finished.
func (e *soEngine) executeBackfill(ctx context.Context, _ bool) error {
	// A recheck ignores every prior completion proof until one copy finishes.
	recheck := e.backfillRecheck.Load()

	// Repeat only after a World notification, with no read held while idle.
	for {
		// Subscribe before capture, so a changed head schedules another copy.
		var waitCh <-chan struct{}
		e.c.writeBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		if err := e.copyBackfill(ctx, recheck); err != nil {
			return err
		}
		recheck = false

		// The completed copy released its read before waiting or retiring.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// copyBackfill pins one durable World until its copy and outcome publication
// return. The engine's read reference also keeps controller teardown from
// closing the block engine while this copy is using its page view.
func (e *soEngine) copyBackfill(ctx context.Context, recheck bool) error {
	// Keep the captured read through copying and publication, including errors.
	head, roots, seqno, release, err := e.backfillRoots(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Copy outside the controller writer, reporting unavailable peer blocks.
	le := e.c.le
	start := time.Now()
	var holes uint32
	missing := func(ref *block.BlockRef) {
		holes++
		le.WithField("block", ref.MarshalString()).Warn("backfill found a block no peer holds")
	}
	opts := &RetainOptions{Missing: missing, Recheck: recheck}
	if recheck {
		opts.Visited = recheckProgress(le, start)
	}
	if err := copyWorlds(ctx, e.so, backfillProofStoreID, roots, opts); err != nil {
		return err
	}

	// Rechecks clear only after their complete walk; stale lifetimes cannot
	// publish a completion even if the last block arrived during cancellation.
	fields := logrus.Fields{"dur": time.Since(start).String(), "holes": holes}
	if recheck {
		e.backfillRecheck.Store(false)
		le.WithFields(fields).Info("rechecked the backfilled World")
	} else {
		le.WithFields(fields).Debug("backfilled the World")
	}
	return e.publishBackfill(ctx, &BackfillState{Root: head, Holes: holes, Seqno: seqno})
}

// recheckProgressInterval is how often a recheck logs its progress.
const recheckProgressInterval = time.Minute

// recheckProgress returns a Visited callback that counts the blocks a recheck
// walks and logs the count and bytes about once per recheckProgressInterval.
// CopyGraph calls Visited from one goroutine.
func recheckProgress(le *logrus.Entry, start time.Time) func(*block.BlockRef, []byte) {
	var blocks, size uint64
	next := start.Add(recheckProgressInterval)
	return func(_ *block.BlockRef, data []byte) {
		blocks++
		size += uint64(len(data))
		if now := time.Now(); now.After(next) {
			next = now.Add(recheckProgressInterval)
			le.WithFields(logrus.Fields{
				"blocks": blocks,
				"bytes":  size,
				"dur":    now.Sub(start).Round(time.Second).String(),
			}).Info("rechecking the backfilled World")
		}
	}
}

// publishBackfill publishes the outcome of a copy under the writer lock, unless
// SetBackfill stopped the copy first.
func (e *soEngine) publishBackfill(ctx context.Context, state *BackfillState) error {
	// Take the lock SetBackfill holds, then check the copy is still wanted.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "publish-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	e.backfillState.SetValue(state)
	return nil
}

// backfillRoots captures the durable read's root and seqno together, plus
// the physical-store frontier of that World and the retained roots. The caller
// releases the read after copying and publishing, before waiting for more work.
// Retained-root metadata keeps its existing separate named-root semantics.
func (e *soEngine) backfillRoots(ctx context.Context) (*block.BlockRef, []*block.BlockRef, uint64, func(), error) {
	for {
		// Opening and reading a snapshot can do storage I/O. Its existing head
		// pin protects kept pages and stored bytes without taking a new root pin.
		if err := ctx.Err(); err != nil {
			return nil, nil, 0, nil, context.Cause(ctx)
		}
		read, err := e.NewTransaction(ctx, false)
		if err != nil {
			return nil, nil, 0, nil, err
		}

		// Read both values from this one immutable World revision.
		var head *block.BlockRef
		err = read.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			head = cursor.GetRef().GetRootRef().CloneVT()
			return nil
		})
		var seqno uint64
		if err == nil {
			seqno, err = read.GetSeqno(ctx)
		}
		if err != nil {
			read.Discard()
			return nil, nil, 0, nil, err
		}

		// Validate cached durable-head metadata under the controller writer.
		// Receipt settlement alone can precede the durable head's installation.
		unlock, err := e.c.lockSettled(ctx, "backfill")
		if err != nil {
			read.Discard()
			return nil, nil, 0, nil, err
		}
		last := e.lastDurable()
		var installed bool
		e.durable.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			installed = last == nil || e.durable.done >= last.pos
		})
		if !installed || !head.EqualsRef(e.bengine.GetRootRef().GetRootRef()) {
			unlock()
			read.Discard()
			if _, _, err := e.waitDurable(ctx, last); err != nil {
				return nil, nil, 0, nil, err
			}
			continue
		}

		// Capture retained-root metadata under the writer; the read pins the
		// captured user head. Copying and storage work start after unlock.
		roots := []*block.BlockRef{head}
		for _, root := range e.retainedRoots {
			roots = append(roots, root.GetRootRef())
		}
		unlock()
		return head, e.replay.pages.frontier(roots), seqno, read.Discard, nil
	}
}

// GetHead returns one durable read's root and seqno, independently of the
// engine's background retained ownership. It releases that read before return.
func (e *soEngine) GetHead(ctx context.Context) (*block.BlockRef, uint64, error) {
	head, _, seqno, release, err := e.backfillRoots(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	return head, seqno, nil
}

// ReplayWorldsEngine lists the World roots this device still names.
type ReplayWorldsEngine interface {
	// ReplayWorlds returns, newest first and each once, the World after each
	// replayed operation, the replay base, and the retained roots. A repair
	// reads an older copy of a damaged part of the World from them.
	ReplayWorlds(ctx context.Context) ([]*block.BlockRef, error)
}

// ReplayWorlds returns the World roots of the replay and the retained roots,
// newest first.
func (e *soEngine) ReplayWorlds(ctx context.Context) ([]*block.BlockRef, error) {
	// Read them under the writer lock, which guards the replay.
	unlockWriteMtx, err := e.c.lockSettled(ctx, "replay-worlds")
	if err != nil {
		return nil, err
	}
	defer unlockWriteMtx()

	// Collect each position's World from the newest, then the base and the
	// retained roots.
	var roots []*block.BlockRef
	add := func(ref *block.BlockRef) {
		if ref.GetEmpty() || slices.ContainsFunc(roots, ref.EqualsRef) {
			return
		}
		roots = append(roots, ref.Clone())
	}
	for _, v := range slices.Backward(e.replay.positions) {
		add(v.state.GetHeadRef().GetRootRef())
	}
	add(e.replay.base.GetHeadRef().GetRootRef())
	for _, root := range e.retainedRoots {
		add(root.GetRootRef())
	}
	return roots, nil
}

// ReplayFromEngine is a World engine that can replay its operations again
// from one of the Worlds its replay passed through.
type ReplayFromEngine interface {
	// ReplayFrom replays every operation after the World at root again, and
	// installs the World the replay reaches. Without apply it only reports.
	ReplayFrom(ctx context.Context, root *block.BlockRef, apply bool) (*ReplayFromReport, error)
}

// ReplayFromReport reports a ReplayFrom.
type ReplayFromReport struct {
	// Kept is the number of replayed operations before the World at root.
	Kept int
	// Replayed is the number of operations replayed again after it.
	Replayed int
	// Head is the World root installed by an applied replay.
	Head *block.BlockRef
}

// ReplayFrom replays every operation after the World at root again, which
// must be the replay base or the World after a replayed operation, and
// installs the World the replay reaches. Replay writes every page the
// operations change again, so this repairs a World whose later pages are
// missing, as long as the World at root has none missing. When the replay
// fails, the replay keeps its former positions and nothing is installed.
func (e *soEngine) ReplayFrom(ctx context.Context, root *block.BlockRef, apply bool) (*ReplayFromReport, error) {
	// Hold the writer lock once the resolved receipts settle.
	unlockWriteMtx, err := e.c.lockSettled(ctx, "replay-from")
	if err != nil {
		return nil, err
	}
	defer unlockWriteMtx()

	// Find the newest position whose World is root.
	r := e.replay
	kept := -1
	for i, v := range slices.Backward(r.positions) {
		if v.state.GetHeadRef().GetRootRef().EqualsRef(root) {
			kept = i + 1
			break
		}
	}
	if kept < 0 && r.base.GetHeadRef().GetRootRef().EqualsRef(root) {
		kept = 0
	}
	if kept < 0 {
		return nil, errors.Errorf("World %s is not the replay base or the World after a replayed operation", root.MarshalString())
	}
	report := &ReplayFromReport{Kept: kept, Replayed: len(r.positions) - kept}
	if !apply {
		return report, nil
	}

	// Drop the positions after root, keeping them to restore on failure.
	positions, changed, deferred, span, spanLen := r.positions, r.changed, r.deferred, r.span, r.spanLen
	restore := func() {
		r.positions, r.changed, r.deferred, r.span, r.spanLen = positions, changed, deferred, span, spanLen
	}
	r.positions = slices.Clone(positions[:kept])
	r.changed, r.deferred = true, false
	if kept < r.spanLen {
		r.span, r.spanLen = nil, 0
	}

	// Replay to the current operation set, then install the World.
	snap, err := e.admittedState(ctx)
	if err != nil {
		restore()
		return report, err
	}
	if _, _, err := e.reach(ctx, snap, nil, false); err != nil {
		restore()
		return report, errors.Wrap(err, "replay")
	}
	if _, err := e.install(ctx, snap, nil, false); err != nil {
		return report, errors.Wrap(err, "install replayed World")
	}
	report.Head = e.admitted.GetRootRef().GetRootRef().Clone()
	return report, nil
}

// _ proves the served engine implements its copy and replay capabilities.
var (
	_ BackfillEngine     = (*soEngine)(nil)
	_ ReplayWorldsEngine = (*soEngine)(nil)
	_ ReplayFromEngine   = (*soEngine)(nil)
)
