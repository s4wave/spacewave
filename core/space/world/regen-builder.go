package space_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// regenBuild is this device's build of a pin, or the refusal every replica
// meets building it.
type regenBuild struct {
	// pin is the kv World built.
	pin *block.BlockRef
	// built is its node graph World, nil when invalid is set.
	built *block.BlockRef
	// invalid is the build's world_block.ErrRegenInvalid failure.
	invalid string
}

// regenBuilder builds the World a regeneration pinned off the replay path,
// so the switch carries a prebuilt root forward by a bounded diff and the
// writers wait only for that (invariant 2 of the apply). It follows the
// installed head's pin: a new pin starts a build and cancels one of an older
// pin, and no pin releases what it held.
//
// Invariant: a published build is held under the regen-build root, a
// HeldRoots block naming the pin and the build, from before it is published
// until the pin leaves the installed head. A switch takes the pin out of the
// state that holds its result under the retained root, and that state
// installs only after holdApplied held it, so the collector frees no page of
// a build a switch reads or installed.
type regenBuilder struct {
	// a is the adapter.
	a *Adapter
	// routine builds the pin its state names, a head bound to the bucket
	// whose root is empty while no pin is held.
	routine *routine.StateRoutineContainer[*bucket.ObjectRef]
	// result is the last finished build, nil when there is none. a.bcast
	// guards it and broadcasts its changes.
	result *regenBuild
}

// newRegenBuilder returns the builder of a's regenerations.
func newRegenBuilder(a *Adapter) *regenBuilder {
	b := &regenBuilder{a: a}
	b.routine = routine.NewStateRoutineContainerWithLoggerVT[*bucket.ObjectRef](
		a.le.WithField("routine", "world-regen-build"),
		routine.WithRetry(backfillBackoff),
	)
	b.routine.SetStateRoutine(b.execute)
	return b
}

// start publishes the build an earlier run held and runs builds under ctx.
// Execute calls it before the driver steps, and follow afterwards.
func (b *regenBuilder) start(ctx context.Context) error {
	if err := b.load(ctx); err != nil {
		return err
	}
	b.routine.SetContext(ctx, false)
	return nil
}

// follow sets the build to the installed head's pin until ctx ends. Execute
// runs it beside the driver, so start-up replay goes on while a build resumes.
// Until the first head installs, the pin is unknown, and follow leaves the
// build an earlier run held: releasing it then would drop a build the head
// still pins.
func (b *regenBuilder) follow(ctx context.Context) error {
	for {
		var head *bucket.ObjectRef
		var waitCh <-chan struct{}
		b.a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			switch pin, _, _ := heldRegen(b.a.headState); {
			case b.a.headState == nil:
			case pin != nil:
				head = b.a.bucketRef(withRoot(b.a.headState, pin).GetHeadRef())
			default:
				head = b.a.bucketRef(nil)
			}
		})
		if head != nil {
			b.routine.SetState(head)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// load publishes the build the regen-build root holds. An unreadable root is
// no build: load logs it and leaves the root held, so the Space opens and the
// next build of a pin replaces the root.
func (b *regenBuilder) load(ctx context.Context) error {
	// Read the root's block under the root lock.
	release, err := b.a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	data, err := b.a.readRootBlock(ctx, RootRegenBuild, b.a.held[RootRegenBuild])
	release()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}

	// Decode the pin and build it names.
	msg := &HeldRoots{}
	if err == nil {
		err = msg.UnmarshalVT(data)
	}
	if err != nil {
		b.a.le.WithError(err).Warn("held regeneration build is not readable, holding no build")
		return nil
	}

	// Publish them.
	if roots := msg.GetRoots(); len(roots) == 2 {
		b.set(&regenBuild{pin: roots[0], built: roots[1]})
	}
	return nil
}

// lookup returns this device's build of pin, or its refusal, or neither when
// it holds none. The replay driver calls it at a switch.
func (b *regenBuilder) lookup(pin *block.BlockRef) (built *block.BlockRef, invalid string) {
	res := b.get()
	if res == nil || !res.pin.EqualsRef(pin) {
		return nil, ""
	}
	return res.built, res.invalid
}

// waitPinned waits until the installed head holds a pin and this device
// finished building it, and returns the build. It returns nil once the head
// holds no pin, the pin switched or released by any device.
func (b *regenBuilder) waitPinned(ctx context.Context) (*regenBuild, error) {
	var out *regenBuild
	err := b.a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		pin, _, _ := heldRegen(b.a.headState)
		if pin == nil {
			out = nil
			return true, nil
		}
		out = b.result
		return out != nil && out.pin.EqualsRef(pin), nil
	})
	return out, err
}

// get returns the last finished build.
func (b *regenBuilder) get() *regenBuild {
	var res *regenBuild
	b.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		res = b.result
	})
	return res
}

// set publishes res as the last finished build.
func (b *regenBuilder) set(res *regenBuild) {
	b.a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		b.result = res
		broadcast()
	})
}

// execute builds the pin head names and holds the result, or releases the
// held build when head names none.
func (b *regenBuilder) execute(ctx context.Context, head *bucket.ObjectRef) error {
	// With no pin, stop publishing the held build, then release it, so no
	// replayed switch looks up a build the root no longer holds.
	pin := head.GetRootRef()
	if pin.GetEmpty() {
		b.set(nil)
		return b.a.moveRoots(ctx, nil, block.NamedRoot{Name: RootRegenBuild})
	}

	// Keep a build of this pin.
	if res := b.get(); res != nil && res.pin.EqualsRef(pin) {
		return nil
	}

	// Build it under a stage, held until the root holds it, yielding to the
	// local writers between slices.
	start := time.Now()
	built, err := b.build(ctx, head)
	switch {
	case err != nil && errors.Is(err, world_block.ErrRegenInvalid) && !isReplayGap(err):
		b.set(&regenBuild{pin: pin, invalid: err.Error()})
		return nil
	case err != nil:
		return err
	}
	b.a.le.WithField("dur", time.Since(start).String()).Debug("built the pinned World off the replay path")
	b.set(&regenBuild{pin: pin, built: built})
	return nil
}

// build writes the node graph of the pin at head under a stage on the block
// store, holds it under the regen-build root and returns it. The pin's pages
// are on the block store: holdApplied stored them when it held the pin.
func (b *regenBuilder) build(ctx context.Context, head *bucket.ObjectRef) (*block.BlockRef, error) {
	// Open a stage and a cursor over it.
	ctx = block.WithBackground(ctx)
	store, releaseStage, err := block.OpenStage(ctx, b.a.pages.StoreOps)
	if err != nil {
		return nil, err
	}
	defer releaseStage()
	cursor, err := b.a.newCursor(ctx, store, head)
	if err != nil {
		return nil, err
	}
	defer cursor.Release()

	// Build the pin.
	pin := head.GetRootRef()
	steps := world_block.NewOrderedRegen(cursor, store, b.a.regenConfig(regenSpillBuild))
	built, err := steps.Build(ctx, pin, b.a.turns.Yield)
	if err != nil {
		return nil, err
	}

	// Hold both under one block before the stage ends.
	refs := []*block.BlockRef{pin, built}
	entry, err := refsEntry(&HeldRoots{Roots: refs}, refs)
	if err != nil {
		return nil, err
	}
	release, err := b.a.lockRoots(ctx, refs...)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := b.a.moveRootsLocked(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootRegenBuild, Ref: entry.Ref}); err != nil {
		return nil, err
	}
	return built, nil
}
