package space_world

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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
	// while backfill is off or before its first copy finishes. Its root and
	// seqno are the installed checkpoint's World and seqno, or the installed
	// head's while the copy follows the head.
	GetBackfillState() ccontainer.Watchable[*BackfillState]
	// GetHead returns the installed World root and its seqno.
	GetHead(ctx context.Context) (*block.BlockRef, uint64, error)
	// SetBackfill chooses whether this device copies the installed
	// checkpoint's World and the retained roots into its local store in the
	// background, at each checkpoint, holding them under the backfill root.
	// The choice persists on this device.
	SetBackfill(ctx context.Context, enabled bool) error
	// SetBackfillFollowsHead chooses whether the copy follows the installed
	// head instead of the checkpoint, copying again at each head. A successor
	// proving its copy during a handover follows the head, since its proof
	// must name the owner's head. The choice lasts for this engine only.
	SetBackfillFollowsHead(follow bool)
	// RecheckBackfill restarts the copy so it reads every block of the
	// covered roots again and fetches each one the store lacks. It returns
	// once the recheck starts; GetBackfillState reports its outcome.
	RecheckBackfill(ctx context.Context) error
}

// spaceBackfill copies a Space's installed checkpoint World and its retained
// roots into the local store while this device's backfill choice is on, and
// holds them under the backfill root.
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
	// followHead is set while the copy follows the installed head instead of
	// the checkpoint. Written under the adapter's bcast.
	followHead bool
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
	if enabled {
		return nil
	}

	// Release a root an earlier run left held when backfill turned off.
	return b.a.moveRoots(ctx, nil, block.NamedRoot{Name: RootBackfill})
}

// set stores the choice and starts or stops the copy. Stopping clears its
// outcome.
func (b *spaceBackfill) set(ctx context.Context, enabled bool) error {
	// Store the choice.
	if err := b.write(ctx, enabled); err != nil {
		return err
	}

	// Publish it and start or stop the copy. Stopping waits for the copy to
	// exit, since one already moving its root lands it, then releases what
	// the copy held and clears its outcome.
	b.choice.SetValue(enabled)
	exited, _, _, _ := b.routine.SetState(enabled)
	if enabled {
		return nil
	}
	if exited != nil {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-exited:
		}
	}
	b.state.SetValue(nil)
	return b.a.moveRoots(ctx, nil, block.NamedRoot{Name: RootBackfill})
}

// setFollowHead chooses whether the copy follows the installed head, and wakes
// the copy when the choice changed.
func (b *spaceBackfill) setFollowHead(follow bool) {
	b.a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if b.followHead != follow {
			b.followHead = follow
			broadcast()
		}
	})
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

// execute copies the installed checkpoint's World and the installed head's
// retained roots, then again whenever either changes, publishing the outcome
// of each copy. The pages of the Worlds after the checkpoint are stored at the
// next checkpoint, so backfill copies at checkpoints rather than at every
// head, and an operation above the checkpoint reads its blocks on demand
// until then. While the copy follows the head, it copies the installed
// head's World in place of the checkpoint's, at each head.
func (b *spaceBackfill) execute(ctx context.Context, _ bool) error {
	var copied []*block.BlockRef
	var copiedSeqno uint64
	var published bool
	for {
		// Capture the roots and the wait for their next change together.
		var root *block.BlockRef
		var seqno uint64
		var roots []*block.BlockRef
		var recheck, ready bool
		var waitCh <-chan struct{}
		b.a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Take the recheck request and the next wake.
			recheck, waitCh = b.recheck, getWaitCh()

			// Copy the checkpoint, or the head while following it.
			root, seqno = b.a.checkpoint.GetHeadRef().GetRootRef(), b.a.checkpointSeqno
			ready = b.a.installed
			if b.followHead {
				root, seqno = b.a.head.Root, b.a.head.Seqno
				ready = !root.GetEmpty()
			}

			// Hold the World root and every retained root.
			if !root.GetEmpty() {
				roots = append(roots, root)
			}
			for _, retained := range b.a.headState.GetRetainedRoots() {
				roots = append(roots, retained.GetRootRef())
			}
		})

		// Copy and hold them when they changed, then publish the outcome.
		changed := !published || seqno != copiedSeqno || !slices.EqualFunc(roots, copied, refEqual)
		if ready && (recheck || changed) {
			holes, err := b.copy(ctx, roots, recheck)
			if err != nil {
				return err
			}
			b.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				b.recheck = b.recheck && !recheck
			})
			if ctx.Err() != nil {
				return context.Canceled
			}
			copied, copiedSeqno, published = roots, seqno, true
			b.state.SetValue(&BackfillState{Root: root, Holes: holes, Seqno: seqno})
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// copy copies the graph of each root into the local store and holds them all
// under the backfill root, and returns the number of blocks no reachable store
// held. A copy skips each subtree its stage holds, which a completion proof
// on the local store lets it hold; a recheck reads every block again.
//
// Invariant: each copied block is held from before it is stored until a later
// copy's root move takes the backfill root off it, first by the stage the copy
// writes under and then by the backfill root, so the collector frees no block
// of the copied World or a retained root while backfill holds it.
func (b *spaceBackfill) copy(ctx context.Context, roots []*block.BlockRef, recheck bool) (uint32, error) {
	// Release the root when there is nothing to copy.
	if len(roots) == 0 {
		return 0, b.a.moveRoots(ctx, nil, block.NamedRoot{Name: RootBackfill})
	}

	// Copy and hold each root, counting the blocks no store held.
	start := time.Now()
	var holes uint32
	opts := block.GraphCopyOptions{
		Forget: true,
		Missing: func(_ context.Context, ref *block.BlockRef) error {
			holes++
			b.a.le.WithField("block", ref.MarshalString()).Warn("backfill found a block no peer holds")
			return nil
		},
	}
	if err := b.a.copyAndHold(ctx, RootBackfill, roots, opts, recheck); err != nil {
		return 0, err
	}
	b.a.le.WithField("dur", time.Since(start).String()).WithField("holes", holes).Debug("backfilled the World")
	return holes, nil
}

// read reads this device's backfill choice.
func (b *spaceBackfill) read(ctx context.Context) (bool, error) {
	_, found, err := b.a.readLocal(ctx, backfillStoreID, backfillKey)
	return found, err
}

// write stores this device's backfill choice.
func (b *spaceBackfill) write(ctx context.Context, enabled bool) error {
	var value []byte
	if enabled {
		value = []byte{1}
	}
	return b.a.writeLocal(ctx, backfillStoreID, backfillKey, value)
}
