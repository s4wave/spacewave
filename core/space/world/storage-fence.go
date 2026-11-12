package space_world

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// storageReclaimDelay is the wait after a head change before a storage
// reclaim pass, so the Volume releases what the change replaced first.
const storageReclaimDelay = 5 * time.Minute

// storageReclaimInterval is the minimum time between reclaim requests. A pass
// lists the storage backend's catalog.
const storageReclaimInterval = time.Hour

// defaultFenceTimeout is the longest a fence waits for every writer device to
// answer. A device offline that long fails the pass, and the next request
// tries again.
const defaultFenceTimeout = 10 * time.Minute

// fenceRootPrefix prefixes the root holding a pass's live roots on this
// device while the pass runs.
const fenceRootPrefix = "fence:"

// errFenceUnanswered is returned when a writer device did not answer a fence
// in time.
var errFenceUnanswered = errors.New("a writer device did not answer the storage fence")

// spaceFence is a fence the driver applied: the writers that must answer it
// and the answers so far.
type spaceFence struct {
	// writers are the participants that could write when the fence was
	// placed, each of which must answer.
	writers []string
	// answers are the roots each writer answered with, by peer ID.
	answers map[string][]*block.BlockRef
}

// ReclaimFence places a fence and returns the union of every writer device's
// held roots, for a storage reclaim pass over the Space's shared backend. With
// copyRoots, it also copies the graph of each root into the local store and
// holds it under the pass's fence root until the pass ends, for a backend that
// judges liveness by what the local store holds.
//
// Invariants. The fence decides what a reclaim pass may delete, so it keeps
// every block it is unsure of:
//
//  1. It returns only after every participant that could write when the
//     fence was placed answered that fence. A device the trimming roster
//     dropped still answers: it may still upload or reference blocks.
//  2. Only an answer naming this pass's fence counts. A root set answering an
//     earlier fence says nothing of what the device holds now.
//  3. A device answers after every operation it started before it saw the
//     fence, through its operation queue, so its answer covers each block
//     such an operation names. A device that joined after the fence was
//     placed started its uploads after the backend listed its catalog, which
//     the backend does before it asks for the fence, so the pass does not
//     judge them and need not wait for it.
//  4. An answer lists every root the device holds in the Space's bucket:
//     the adapter's roots, reader pins and stages.
//  5. A writer that does not answer within the timeout fails the pass, which
//     then deletes nothing.
//  6. No pass frees anything while frees are off: the Volume collector's
//     Frees switch and the Spacewave pack reclaim switch (SetPackReclaim) are
//     both off by default, so the fence answers but no backend deletes by it
//     until an operator turns them on.
func (a *Adapter) ReclaimFence(ctx context.Context, copyRoots bool) ([]*block.BlockRef, error) {
	// Place the fence after this device's admitted operations.
	pass := ulid.NewULID()
	adm, err := a.enqueueOp(&WorldOp{Body: &WorldOp_Fence{Fence: &FenceOp{Pass: pass}}})
	if err != nil {
		return nil, err
	}

	// Wait for the driver to apply it, and read its hash.
	if _, err := adm.WaitDecided(ctx); err != nil {
		return nil, err
	}
	var fence []byte
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		fence = adm.hash
	})

	// Wait for every writer's answer.
	timeout := a.opts.FenceTimeout
	if timeout == 0 {
		timeout = defaultFenceTimeout
	}
	ctx, cancel := context.WithTimeoutCause(ctx, timeout, errFenceUnanswered)
	defer cancel()
	live, err := a.waitAnswers(ctx, fence)
	if err != nil {
		return nil, err
	}
	if !copyRoots {
		return live, nil
	}

	// Copy and hold the live roots until the pass ends.
	if err := a.holdFence(ctx, pass, live); err != nil {
		return nil, err
	}
	return live, nil
}

// waitAnswers waits until every writer the fence names answered it and
// returns the union of their roots.
func (a *Adapter) waitAnswers(ctx context.Context, fence []byte) ([]*block.BlockRef, error) {
	// Collect the answers once every writer gave one.
	var live []*block.BlockRef
	err := a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		// Find the fence.
		live = nil
		f := a.fences[string(fence)]
		if f == nil {
			return false, nil
		}

		// Union each writer's answer, stopping at a missing one.
		for _, writer := range f.writers {
			roots, ok := f.answers[writer]
			if !ok {
				return false, nil
			}
			live = append(live, roots...)
		}
		return true, nil
	})
	if err != nil {
		return nil, context.Cause(ctx)
	}
	return dedupRefs(live), nil
}

// holdFence copies the graph of each live root into the local store, skipping
// each subtree the copy's stage holds, and holds them all under the pass's
// fence root.
func (a *Adapter) holdFence(ctx context.Context, pass string, live []*block.BlockRef) error {
	return a.copyAndHold(ctx, fenceRootPrefix+pass, live, block.GraphCopyOptions{Forget: true}, false)
}

// copyAndHold copies the graph of each root from the kept pages and the read
// path into the SharedObject's block store with opts, under a stage that holds
// each copied block until the named root name holds one block naming every
// root. It skips only the subtrees the stage holds, as block.StageCopy
// describes, unless recheck reads every block again. The stage ends after the
// root moved, so no copied block is unheld in between.
//
// The root moves on the kept pages' store, not through the kept pages, so the
// copy leaves every kept page in memory. A copy's root releases what it held
// at its next copy or pass, whether or not the adapter still reads it, so a
// kept page the move took out of memory would be held by nothing once the copy
// moved on. A live World's pages leave memory only under the replay base or a
// history, whose trim keeps every live World.
func (a *Adapter) copyAndHold(ctx context.Context, name string, roots []*block.BlockRef, opts block.GraphCopyOptions, recheck bool) error {
	// Copy under a stage until the root holds them.
	c, err := block.OpenStageCopy(ctx, a.so.GetBlockStore(), recheck)
	if err != nil {
		return err
	}
	defer c.Release()
	for _, root := range roots {
		if err := c.Copy(ctx, a.pages, root, opts); err != nil {
			return errors.Wrapf(err, "copy root %s", root.MarshalString())
		}
	}
	if err := c.Flush(ctx); err != nil {
		return err
	}

	// Hold them under one block, which the copy stored all of.
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return err
	}
	release, err := a.lockRootMtx(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.moveRootsOn(ctx, a.pages.StoreOps, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: name, Ref: entry.Ref})
}

// releaseFences releases every fence root this device holds.
func (a *Adapter) releaseFences(ctx context.Context) error {
	// Find the fence roots.
	release, err := a.lockRootMtx(ctx)
	if err != nil {
		return err
	}
	defer release()
	var roots []block.NamedRoot
	for name := range a.held {
		if len(name) > len(fenceRootPrefix) && name[:len(fenceRootPrefix)] == fenceRootPrefix {
			roots = append(roots, block.NamedRoot{Name: name})
		}
	}

	// Release them.
	return a.moveRootsLocked(ctx, nil, roots...)
}

// noteFence records a fence or an answer the driver applied, and queues this
// device's answer to a fence it is a writer of. The driver goroutine calls it.
func (a *Adapter) noteFence(ctx context.Context, op *sobject.OrderedOperation, pos spacePosition) error {
	// Only an applied fence or answer counts.
	if pos.reason != "" || op.Data == nil {
		return nil
	}
	wop := &WorldOp{}
	if err := wop.UnmarshalVT(op.Data); err != nil {
		return nil
	}
	switch body := wop.GetBody().(type) {
	case *WorldOp_Fence:
		return a.noteFenceOp(ctx, op)
	case *WorldOp_RootSet:
		a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if f := a.fences[string(body.RootSet.GetFence())]; f != nil {
				f.answers[op.Inner.GetPeerId()] = body.RootSet.GetRoots()
				broadcast()
			}
		})
	}
	return nil
}

// noteFenceOp records the writers a fence waits for, and queues this device's
// answer once per fence.
func (a *Adapter) noteFenceOp(ctx context.Context, op *sobject.OrderedOperation) error {
	// Record the writers when first seen.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return err
	}
	var seen bool
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if _, seen = a.fences[string(op.Hash)]; !seen {
			a.fences[string(op.Hash)] = &spaceFence{writers: cfg.Writers(), answers: make(map[string][]*block.BlockRef)}
		}
	})
	self := a.so.GetPeerID().String()
	if seen || !slices.Contains(cfg.Writers(), self) {
		return nil
	}

	// Answer with every root the device holds in the Space's bucket.
	var roots []*block.BlockRef
	err = a.pages.ListRoots(ctx, "", func(root block.NamedRoot) error {
		roots = append(roots, root.Ref)
		return nil
	})
	if err != nil {
		return err
	}

	// Queue it after the operations this device started.
	_, err = a.enqueueOp(&WorldOp{Body: &WorldOp_RootSet{RootSet: &RootSetOp{Fence: op.Hash, Roots: dedupRefs(roots)}}})
	return err
}

// executeStorageReclaim asks the block store for a storage reclaim pass
// storageReclaimDelay after startup and after each head change, and at the
// time the block store says the next pass comes due, at least
// storageReclaimInterval apart. Only the checkpointer asks, and not while it
// hands over.
func (a *Adapter) executeStorageReclaim(ctx context.Context) error {
	// Track the pending request and the time of the previous one.
	var timer *time.Timer
	var at, last time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	schedule := func(next time.Time) {
		if earliest := last.Add(storageReclaimInterval); next.Before(earliest) {
			next = earliest
		}
		if timer != nil {
			if !next.Before(at) {
				return
			}
			timer.Stop()
		}
		at, timer = next, time.NewTimer(time.Until(next))
	}
	schedule(time.Now().Add(storageReclaimDelay))

	// Reschedule after each head change, and run a pass when due.
	var head world.Head
	for {
		var waitCh <-chan struct{}
		var changed bool
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = !a.head.Root.EqualsRef(head.Root)
			head, waitCh = a.head, getWaitCh()
		})
		if changed {
			schedule(time.Now().Add(storageReclaimDelay))
		}
		var timerCh <-chan time.Time
		if timer != nil {
			timerCh = timer.C
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		case <-timerCh:
			timer, last = nil, time.Now()
			next, err := a.reclaimStorage(ctx)
			if err != nil {
				a.le.WithError(err).Warn("storage reclaim pass failed")
			}
			if !next.IsZero() {
				schedule(next)
			}
		}
	}
}

// reclaimStorage runs one pass as the checkpointer, then releases the fence
// roots it held.
func (a *Adapter) reclaimStorage(ctx context.Context) (time.Time, error) {
	// Only the checkpointer that is not handing over reclaims.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return time.Time{}, err
	}
	self := a.so.GetPeerID().String()
	if cfg.Checkpointer() != self {
		return time.Time{}, nil
	}
	own, err := a.ownershipOf(ctx, snap)
	if err != nil || own.Departing(self) {
		return time.Time{}, err
	}

	// Run the pass, then release what its fence held.
	next, err := a.so.GetBlockStore().ReclaimStorage(ctx, a.ReclaimFence)
	if relErr := a.releaseFences(ctx); err == nil {
		err = relErr
	}
	return next, err
}

// dedupRefs returns refs without empty or repeated references.
func dedupRefs(refs []*block.BlockRef) []*block.BlockRef {
	seen := make(map[string]struct{}, len(refs))
	out := make([]*block.BlockRef, 0, len(refs))
	for _, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	return out
}
