package space_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// backfillStoreID is the local state store holding this device's backfill
// choice for the World.
const backfillStoreID = "world-backfill"

// backfillKey is the key of the backfill choice in its store. It is present
// when backfill is on.
var backfillKey = []byte("enabled")

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
// of the World. Without backfill the device reads blocks on demand.
//
// The served engine implements it, not the BusEngine that world.StartEngine
// returns: assert it inside world.AccessEngine.
type BackfillEngine interface {
	// GetBackfill watches this device's backfill choice.
	GetBackfill() ccontainer.Watchable[bool]
	// GetBackfillState watches the outcome of the last finished copy, nil
	// while backfill is off or before its first copy finishes.
	GetBackfillState() ccontainer.Watchable[*BackfillState]
	// GetHead returns the installed World root and its seqno. A copy that
	// covers the same root and seqno holds the same World.
	GetHead(ctx context.Context) (*block.BlockRef, uint64, error)
	// SetBackfill chooses whether this device copies the installed World and
	// the retained roots into its local store in the background, holding them
	// under the backfill root. The choice persists on this device.
	SetBackfill(ctx context.Context, enabled bool) error
	// RecheckBackfill restarts the copy so it reads every block of the
	// covered roots again and fetches each one the store lacks. It returns
	// once the recheck starts; GetBackfillState reports its outcome.
	RecheckBackfill(ctx context.Context) error
}

// spaceBackfill copies a Space's installed World and its retained roots into
// the local store while this device's backfill choice is on, and holds them
// under the backfill root.
type spaceBackfill struct {
	// a is the adapter.
	a *Adapter
	// choice is this device's backfill choice.
	choice *ccontainer.CContainer[bool]
	// state is the outcome of the last finished copy while backfill is on.
	state *ccontainer.CContainer[*BackfillState]
	// routine runs the copy while the choice is on.
	routine *routine.StateRoutineContainer[bool]
	// recheck is set while the next or running copy is a recheck. Written
	// under the adapter's bcast.
	recheck bool
}

// newSpaceBackfill returns the backfill of a's World.
func newSpaceBackfill(a *Adapter) *spaceBackfill {
	b := &spaceBackfill{
		a:      a,
		choice: ccontainer.NewCContainer(false),
		state:  ccontainer.NewCContainerVT[*BackfillState](nil),
	}
	b.routine = routine.NewStateRoutineContainer[bool](
		nil,
		routine.WithExitLogger(a.le.WithField("routine", "world-backfill")),
		routine.WithRetry(backfillBackoff),
	)
	b.routine.SetStateRoutine(b.execute)
	return b
}

// start loads the stored choice and runs the copy for ctx's lifetime.
func (b *spaceBackfill) start(ctx context.Context) error {
	// Load the choice, then run the copy under it.
	enabled, err := b.read(ctx)
	if err != nil {
		return err
	}
	b.choice.SetValue(enabled)
	b.routine.SetContext(ctx, false)
	b.routine.SetState(enabled)
	return nil
}

// set stores the choice and starts or stops the copy. Stopping clears its
// outcome.
func (b *spaceBackfill) set(ctx context.Context, enabled bool) error {
	// Store the choice.
	if err := b.write(ctx, enabled); err != nil {
		return err
	}

	// Publish it and start or stop the copy. Stopping releases what the
	// copy held and clears its outcome.
	b.choice.SetValue(enabled)
	b.routine.SetState(enabled)
	if enabled {
		return nil
	}
	b.state.SetValue(nil)
	return b.a.moveRoots(ctx, nil, block.NamedRoot{Name: RootBackfill})
}

// recheckNow restarts the copy as a recheck.
func (b *spaceBackfill) recheckNow() error {
	// A recheck needs backfill on.
	if !b.choice.GetValue() {
		return errors.New("backfill is off")
	}

	// Mark the next copy a recheck, clear the outcome and restart.
	b.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		b.recheck = true
	})
	b.state.SetValue(nil)
	b.routine.RestartRoutine()
	return nil
}

// execute copies the installed World and the retained roots, then again after
// each installed head, publishing the outcome of each copy.
func (b *spaceBackfill) execute(ctx context.Context, _ bool) error {
	for {
		// Capture the head and the wait for the next one together.
		var head *block.BlockRef
		var seqno uint64
		var roots []*block.BlockRef
		var recheck bool
		var waitCh <-chan struct{}
		b.a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			head, seqno, recheck, waitCh = b.a.head.Root, b.a.head.Seqno, b.recheck, getWaitCh()
			for _, root := range b.a.headState.GetRetainedRoots() {
				roots = append(roots, root.GetRootRef())
			}
		})
		if head.GetEmpty() {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
			}
			continue
		}

		// Copy and hold them, then publish the outcome.
		holes, err := b.copy(ctx, append([]*block.BlockRef{head}, roots...), recheck)
		if err != nil {
			return err
		}
		b.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			b.recheck = b.recheck && !recheck
		})
		if ctx.Err() != nil {
			return context.Canceled
		}
		b.state.SetValue(&BackfillState{Root: head, Holes: holes, Seqno: seqno})

		// Wait for the next head.
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// copy copies the graph of each root into the local store under a stage,
// then holds them all under the backfill root, and returns the number of
// blocks no reachable store held. A recheck reads every block again; a copy
// skips each subtree the local store already holds.
func (b *spaceBackfill) copy(ctx context.Context, roots []*block.BlockRef, recheck bool) (uint32, error) {
	// Write the copy under a stage until the root holds it.
	store := b.a.so.GetBlockStore()
	staged, release, err := block.OpenStage(ctx, store)
	if err != nil {
		return 0, err
	}
	defer release()

	// Copy each root from the kept pages and the read path.
	start := time.Now()
	var holes uint32
	opts := &block.GraphCopyOptions{
		Forget: true,
		Missing: func(_ context.Context, ref *block.BlockRef) error {
			holes++
			b.a.le.WithField("block", ref.MarshalString()).Warn("backfill found a block no peer holds")
			return nil
		},
	}
	if !recheck {
		opts.Known = store.GetBlockExistsBatch
	}
	for _, root := range roots {
		if err := block.CopyGraph(ctx, b.a.pages, staged, root, opts); err != nil {
			return 0, errors.Wrapf(err, "copy World %s", root.MarshalString())
		}
	}

	// Hold them under one block naming every root.
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return 0, err
	}
	if err := b.a.moveRoots(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootBackfill, Ref: entry.Ref}); err != nil {
		return 0, err
	}
	b.a.le.WithField("dur", time.Since(start).String()).WithField("holes", holes).Debug("backfilled the World")
	return holes, nil
}

// read reads this device's backfill choice.
func (b *spaceBackfill) read(ctx context.Context) (bool, error) {
	// Open the choice's store.
	store, release, err := b.a.so.AccessLocalStateStore(ctx, backfillStoreID, nil)
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

// write stores this device's backfill choice.
func (b *spaceBackfill) write(ctx context.Context, enabled bool) error {
	// Open the choice's store.
	store, release, err := b.a.so.AccessLocalStateStore(ctx, backfillStoreID, nil)
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
