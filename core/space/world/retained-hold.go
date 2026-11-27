package space_world

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// RetainedWaitEnv restores the stop at a retained root this device cannot hold
// when set to "off": the install that lands it fails, so replay stops at the
// operation that retained it and steps again after the driver's backoff.
// Otherwise replay continues and the root waits for a source, as retainedHold
// describes. It is read at each use.
const RetainedWaitEnv = "SPACEWAVE_WORLD_RETAINED_WAIT"

// retainedRetryMin and retainedRetryMax bound the retrier's backoff between
// tries of the roots that still wait. retainedRetryMin also spaces tries an
// install wakes, so a stream of installs costs at most one fetch of each
// waiting root per interval.
const (
	retainedRetryMin = 5 * time.Second
	retainedRetryMax = 5 * time.Minute
)

// rootWaitState is why a retained root waits.
type rootWaitState uint8

const (
	// rootUnchecked is a root an earlier run's set lists, which this run has
	// not checked the store for yet.
	rootUnchecked rootWaitState = iota
	// rootMissing is a root the store lacks and no source served at the last
	// try. The health shows it.
	rootMissing
)

// retainedWait is a retained root the store may lack.
type retainedWait struct {
	// root is the retained root.
	root *RetainedRoot
	// state is why it waits.
	state rootWaitState
}

// retainedHold is the state machine of the RootRetained root: the retained set
// the root holds, and each root of it the store may lack.
//
// Each root of the set is held, unchecked or missing. Held is a root whose
// block the store holds, or one a pin brought in. Unchecked is a root an
// earlier run's set lists, which this run has not checked yet. Missing is a
// root the store lacks and no source served at the last try, such as a World
// snapshot whose only copy was on a device that is gone. Only held roots are
// absent from waits.
//
// An install lands the set with every root its state retains, whatever their
// states. Its flush pins each root it does not know waits. A root no source
// serves there becomes missing, and the rest are held. The retrier tries each
// unchecked and missing root off the install path. An install wakes it, and a
// backoff wakes it while roots still wait. A root a source serves becomes held,
// and one no source serves becomes missing. The driver shows the missing roots
// in the Space's health (sobject.RootWaiter).
//
// Invariants:
//
//  1. The set RootRetained holds lists every root the installed state retains,
//     in every state. A missing root is never dropped from it. Only an
//     install of a state that no longer retains the root removes it.
//  2. A held root stays held. No path moves a root from held back to
//     unchecked or missing within a run. Only a load, which knows nothing
//     yet, starts a root as unchecked.
//  3. Nothing is released early. An install releases its flush's pins only
//     after the set that references the roots landed. The retrier releases
//     its pin of a root only after it saw that the landed set still lists the
//     root. The set's reference, which the store recorded when the set
//     landed without the root's block, then holds the block the pin brought
//     in. A root the set no longer lists was released by the install that
//     dropped it.
//  4. An install never waits on a fetch of a root it knows waits. It leaves
//     unchecked and missing roots to the retrier, and the swap stores any
//     kept pages of theirs.
//
// The fields roots and waits change under rootMtx. mtx also guards waits and
// changed, so the driver's report reads them without rootMtx.
type retainedHold struct {
	// roots is the set RootRetained holds, under rootMtx.
	roots []*RetainedRoot
	// wake asks the retrier to try the waits, buffered one.
	wake chan struct{}

	// mtx guards the fields below.
	mtx sync.Mutex
	// waits are the roots of roots the store may lack, in roots' order.
	waits []retainedWait
	// changed is closed when the missing roots change, or nil before a
	// RootWaits call asked for it.
	changed chan struct{}
}

// newRetainedHold returns the state before the roots are read.
func newRetainedHold() retainedHold {
	return retainedHold{wake: make(chan struct{}, 1)}
}

// poke wakes the retrier without waiting.
func (h *retainedHold) poke() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// setWaits replaces waits under the caller's rootMtx hold, and closes changed
// when the missing roots changed. It returns the roots now missing that were
// not before.
func (h *retainedHold) setWaits(waits []retainedWait) (added []*RetainedRoot) {
	// Swap the waits and find the roots newly missing.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	before := missingRoots(h.waits)
	h.waits = waits
	after := missingRoots(waits)
	wasMissing := rootKeys(before)
	for _, root := range after {
		if _, ok := wasMissing[rootKey(root)]; !ok {
			added = append(added, root)
		}
	}

	// Wake the report when the missing roots changed.
	if !slices.EqualFunc(before, after, (*RetainedRoot).EqualVT) && h.changed != nil {
		close(h.changed)
		h.changed = nil
	}
	return added
}

// missingRoots returns the missing roots of waits.
func missingRoots(waits []retainedWait) []*RetainedRoot {
	var out []*RetainedRoot
	for _, w := range waits {
		if w.state == rootMissing {
			out = append(out, w.root)
		}
	}
	return out
}

// rootKey identifies a retained root by its name and ref, as EqualVT compares
// them.
func rootKey(root *RetainedRoot) string {
	return root.GetName() + "\x00" + root.GetRootRef().MarshalString()
}

// rootKeys indexes roots by rootKey.
func rootKeys(roots []*RetainedRoot) map[string]struct{} {
	keys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		keys[rootKey(root)] = struct{}{}
	}
	return keys
}

// waitIndex indexes waits by their root's rootKey.
func waitIndex(waits []retainedWait) map[string]retainedWait {
	index := make(map[string]retainedWait, len(waits))
	for _, w := range waits {
		index[rootKey(w.root)] = w
	}
	return index
}

// refKeys indexes block refs by their string form.
func refKeys(refs []*block.BlockRef) map[string]struct{} {
	keys := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		keys[ref.MarshalString()] = struct{}{}
	}
	return keys
}

// RootWaits returns the retained roots the store lacks and no source served,
// in the order the state lists them, and a channel closed when they change.
func (a *Adapter) RootWaits() ([]*sobject.SORootWait, <-chan struct{}) {
	// Read the missing roots and ask for the change signal.
	h := &a.retained
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if h.changed == nil {
		h.changed = make(chan struct{})
	}
	var out []*sobject.SORootWait
	for _, root := range missingRoots(h.waits) {
		out = append(out, &sobject.SORootWait{Name: root.GetName(), RootRef: root.GetRootRef().MarshalString()})
	}
	return out, h.changed
}

// retainedWaitOff reports whether RetainedWaitEnv restores the stop.
func retainedWaitOff() bool {
	return os.Getenv(RetainedWaitEnv) == "off"
}

// readRetained loads retained roots under the caller's rootMtx hold. Each
// starts unchecked, and the retrier checks that the store holds it.
func (a *Adapter) readRetained(ctx context.Context) error {
	// Read the block the root holds, then decode its roots.
	data, err := a.readRootBlock(ctx, RootRetained, a.held[RootRetained])
	if err != nil || data == nil {
		return err
	}
	msg := &RetainedRootSet{}
	if err := msg.UnmarshalVT(data); err != nil {
		return err
	}
	a.retained.roots = msg.GetRoots()
	if retainedWaitOff() {
		return nil
	}

	// Leave checking them to the retrier.
	waits := make([]retainedWait, len(a.retained.roots))
	for i, root := range a.retained.roots {
		waits[i] = retainedWait{root: root, state: rootUnchecked}
	}
	a.retained.setWaits(waits)
	a.retained.poke()
	return nil
}

// holdRetained makes the RootRetained root hold roots, releasing an empty set,
// and wakes the retrier. A root no source serves stays in the set and waits as
// missing, unless RetainedWaitEnv restores the stop.
func (a *Adapter) holdRetained(ctx context.Context, roots []*RetainedRoot) error {
	// Skip unchanged retained roots; only the driver changes the set. Every
	// install wakes the retrier.
	defer a.retained.poke()
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	same := slices.EqualFunc(roots, a.retained.roots, (*RetainedRoot).EqualVT)
	waits := slices.Clone(a.retained.waits)
	unlock()
	if same {
		return nil
	}

	// Store the retained Worlds' kept pages and pin the roots the store
	// lacks, except those that wait, whose fetch is the retrier's.
	waiting := waitIndex(waits)
	refs := make([]*block.BlockRef, len(roots))
	var reach []*block.BlockRef
	for i, root := range roots {
		refs[i] = root.GetRootRef()
		if _, ok := waiting[rootKey(root)]; !ok || retainedWaitOff() {
			reach = append(reach, refs[i])
		}
	}
	release, unavailable, err := a.lockRootsServed(ctx, reach)
	if err != nil {
		return err
	}
	defer release()
	if len(unavailable) != 0 && retainedWaitOff() {
		return unavailableRootErr(unavailable)
	}

	// Release the name when nothing is retained.
	if len(roots) == 0 {
		if err := a.moveRootsLocked(ctx, nil, block.NamedRoot{Name: RootRetained}); err != nil {
			return err
		}
		a.retained.roots = nil
		a.retained.setWaits(nil)
		return nil
	}

	// Hold every root under one block, whether it waits or not.
	entry, err := refsEntry(&RetainedRootSet{Roots: roots}, refs)
	if err != nil {
		return err
	}
	if err := a.moveRootsLocked(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootRetained, Ref: entry.Ref}); err != nil {
		return err
	}
	a.retained.roots = slices.Clone(roots)

	// Keep the waits of the roots the set still lists, as the retrier left
	// them, and add the roots no source served as missing.
	waiting = waitIndex(a.retained.waits)
	unserved := refKeys(unavailable)
	var next []retainedWait
	for _, root := range roots {
		if _, ok := unserved[root.GetRootRef().MarshalString()]; ok {
			next = append(next, retainedWait{root: root, state: rootMissing})
			continue
		}
		if w, ok := waiting[rootKey(root)]; ok {
			next = append(next, w)
		}
	}
	a.logMissing(a.retained.setWaits(next))
	return nil
}

// retryRetained tries once to hold each waiting root, off the install path,
// and returns whether any root still waits. Its flush pins each root a source
// serves into the store, and may wait on a peer fetch for each.
func (a *Adapter) retryRetained(ctx context.Context) (bool, error) {
	// Read the waits.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return true, err
	}
	tried := slices.Clone(a.retained.waits)
	unlock()
	if len(tried) == 0 {
		return false, nil
	}

	// Pin them outside rootMtx. The pins end only after the waits below
	// moved, so a served root stays held until the set's reference holds it.
	refs := make([]*block.BlockRef, len(tried))
	for i, w := range tried {
		refs[i] = w.root.GetRootRef()
	}
	release, unavailable, err := a.pages.Flush(ctx, refs)
	if err != nil {
		return true, err
	}
	defer release()

	// Take rootMtx again; the pins are released after it.
	unlock, err = a.rootMtx.Lock(ctx)
	if err != nil {
		return true, err
	}
	defer unlock()

	// Move each tried root the landed set still lists: held when served,
	// missing otherwise. An install meanwhile may have dropped a root, and
	// the root it added waits untried.
	triedKeys := waitIndex(tried)
	unserved := refKeys(unavailable)
	var next, served []retainedWait
	for _, w := range a.retained.waits {
		_, wasTried := triedKeys[rootKey(w.root)]
		_, isUnserved := unserved[w.root.GetRootRef().MarshalString()]
		switch {
		case !wasTried:
			next = append(next, w)
		case isUnserved:
			next = append(next, retainedWait{root: w.root, state: rootMissing})
		default:
			served = append(served, w)
		}
	}
	a.logMissing(a.retained.setWaits(next))
	for _, w := range served {
		if w.state == rootMissing {
			a.le.WithField("retained-root", w.root.GetName()).Info("a source served the missing retained root, holding it")
		}
	}
	return len(next) != 0, nil
}

// logMissing reports the roots that became missing.
func (a *Adapter) logMissing(added []*RetainedRoot) {
	for _, root := range added {
		a.le.WithField("retained-root", root.GetName()).
			WithField("root-ref", root.GetRootRef().MarshalString()).
			Warn("no source served a retained root, replay continues and holds it once one does")
	}
}

// executeRetainedRetry retries the waiting roots until ctx ends: when an
// install or a load wakes it, and after a backoff that doubles while roots
// still wait. Tries are at least retainedRetryMin apart.
func (a *Adapter) executeRetainedRetry(ctx context.Context) error {
	backoff := retainedRetryMin
	var timer *time.Timer
	timeout := func() <-chan time.Time {
		if timer == nil {
			return nil
		}
		return timer.C
	}
	for {
		// Wait for a wake or the backoff.
		select {
		case <-ctx.Done():
			return nil
		case <-a.retained.wake:
		case <-timeout():
		}

		// Try the waits, and back off while any still waits.
		waiting, err := a.retryRetained(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			a.le.WithError(err).Warn("retrying the retained roots this device lacks")
		}
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		if waiting || err != nil {
			timer = time.NewTimer(backoff)
			backoff = min(2*backoff, retainedRetryMax)
		} else {
			backoff = retainedRetryMin
		}

		// Space the tries apart.
		space := time.NewTimer(retainedRetryMin)
		select {
		case <-ctx.Done():
			space.Stop()
			return nil
		case <-space.C:
		}
	}
}

// unavailableRootErr is the failure of a flush that found roots no source
// served.
func unavailableRootErr(refs []*block.BlockRef) error {
	return fmt.Errorf("pin root %s: %w", refs[0].MarshalString(), block.ErrUnavailable)
}
