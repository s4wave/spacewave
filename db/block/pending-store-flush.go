package block

import (
	"context"
	"errors"
	"sync"
)

// flushSliceBytes and flushSliceLinks bound the data, and the blocks plus
// the child refs they name, that one put of a Flush writes, so a writer
// waiting on the inner store's writer waits for at most one slice. The inner
// store's cost grows with each block it stores, its child list and stage
// root, and with each child ref, as much as with the bytes.
const (
	flushSliceBytes = 4 << 20
	flushSliceLinks = 2048
)

// Flush stores the kept trees the roots reach under a stage of the inner
// store, in puts of at most flushSliceBytes and flushSliceLinks, and pins
// each root it keeps no block of and the inner store lacks, which brings a
// root another store backs, such as a peer, into the inner store; a root no
// store holds is left to the swap. Until release, SetRoots puts no block the
// flush stored, so a writer that flushes before it takes the lock its root
// moves need holds that lock, and the inner store's writer, only for the
// swap: the stage holds the stored blocks until the swap names them. Each
// slice is marked once stored, so a concurrent flush stores it no second
// time, and a flush that left out another's block keeps that flush's stage
// until its own release. The flushed blocks stay kept, so the Worlds that name them still read
// them from memory, and leave memory with the SetRoots that names them.
// Release after that SetRoots returns. The walks hold the PendingStore's
// lock for at most walkSlice blocks at a time, so readers and writers of kept
// blocks wait only that long.
//
// Cost: one inner put per slice of the kept trees, and one existence read and
// pin per unkept root, all before the caller's lock.
func (s *PendingStore) Flush(ctx context.Context, roots []*BlockRef) (release func(), err error) {
	// Gather the kept trees no open flush stored, and the roots kept nowhere
	// here.
	tree, keys, stopped := s.reachSliced(roots, true)
	s.mtx.Lock()
	var unkept []*BlockRef
	for _, root := range roots {
		if !root.GetEmpty() && s.kept[pendingKey(root)] == nil {
			unkept = append(unkept, root)
		}
	}
	s.mtx.Unlock()

	// Open the stage that holds them until the swap.
	stage, releaseStage, err := OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, err
	}
	var pins []func()
	marked := 0
	cleanup := func() {
		// Unmark, end this flush's reliance on the stages of the blocks it
		// left out, then release the stage and pins once no SetRoots or
		// Flush that left out a flushed block is open: that SetRoots names
		// the block without storing it, so the stage holds it until it
		// lands.
		s.unmark(keys[:marked])
		if stopped {
			s.landed()
		}
		s.afterSkips(func() {
			for _, unpin := range pins {
				unpin()
			}
			releaseStage()
		})
	}

	// Store the trees in slices, children before parents, marking each
	// slice once stored, so a concurrent walk stops at it.
	var batchBytes, batchLinks int
	for i, e := range tree {
		batchBytes, batchLinks = batchBytes+len(e.Data), batchLinks+1+len(e.Refs)
		if batchBytes < flushSliceBytes && batchLinks < flushSliceLinks && i != len(tree)-1 {
			continue
		}
		if _, err := stage.PutBlockBatch(ctx, tree[marked:i+1]); err != nil {
			cleanup()
			return nil, err
		}
		s.mtx.Lock()
		for _, key := range keys[marked : i+1] {
			s.flushed[key]++
		}
		s.mtx.Unlock()
		marked, batchBytes, batchLinks = i+1, 0, 0
	}

	// Pin each unkept root the inner store lacks, copying it in. A root no
	// store holds yet, such as a block the swap itself puts, is left to the
	// swap.
	for _, root := range unkept {
		found, err := s.StoreOps.GetBlockExists(ctx, root)
		if err != nil {
			cleanup()
			return nil, err
		}
		if found {
			continue
		}
		unpin, err := PinRoot(ctx, s.StoreOps, root)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			cleanup()
			return nil, err
		}
		pins = append(pins, unpin)
	}

	// Unmark them and release the stage once.
	var once sync.Once
	return func() { once.Do(cleanup) }, nil
}

// unmark ends one Flush's mark of each of keys.
func (s *PendingStore) unmark(keys []string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, key := range keys {
		if s.flushed[key]--; s.flushed[key] <= 0 {
			delete(s.flushed, key)
		}
	}
}

// walkSlice and dropSlice bound the blocks a walk visits and a drop forgets
// per hold of the PendingStore's lock.
const (
	walkSlice = 1024
	dropSlice = 512
)

// reachSliced returns the kept blocks the roots reach, children before
// parents, with their keys, taking the lock shared for at most walkSlice
// blocks at a time, so a put waits for at most that many. With stopFlushed,
// it neither returns nor descends into a block an open Flush stored, and at
// the first such block it counts the caller as skipping, under the lock that
// saw the mark, and reports stopped: the caller relies on that Flush's stage
// and calls landed once it no longer does. A block kept or dropped between
// slices is seen or missed as of its slice: the trees are immutable, and a
// dropped block is stored.
func (s *PendingStore) reachSliced(roots []*BlockRef, stopFlushed bool) (out []*PutBatchEntry, keys []string, stopped bool) {
	// Hold the lock for the walk, yielding it every walkSlice blocks.
	seen := make(map[string]struct{})
	visits := 0
	s.mtx.RLock()
	defer func() { s.mtx.RUnlock() }()

	// Enter each kept block once.
	type frame struct {
		key   string
		e     *PutBatchEntry
		child int
	}
	enter := func(key string) (*PutBatchEntry, bool) {
		// Yield the lock every walkSlice blocks.
		if visits++; visits%walkSlice == 0 {
			s.mtx.RUnlock()
			s.mtx.RLock()
		}

		// Visit a kept block once, stopping at a flushed one.
		if _, ok := seen[key]; ok {
			return nil, false
		}
		e := s.kept[key]
		if e == nil {
			return nil, false
		}
		seen[key] = struct{}{}
		if stopFlushed && s.flushed[key] > 0 {
			if !stopped {
				stopped = true
				s.skipping.Add(1)
			}
			return nil, false
		}
		return e, true
	}

	// Walk depth first, emitting each block after its kept children.
	for _, root := range roots {
		if root.GetEmpty() {
			continue
		}
		key := pendingKey(root)
		e, ok := enter(key)
		if !ok {
			continue
		}
		stack := []frame{{key: key, e: e}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.child < len(top.e.Refs) {
				ref := top.e.Refs[top.child]
				top.child++
				ckey := pendingKey(ref)
				if ce, ok := enter(ckey); ok {
					stack = append(stack, frame{key: ckey, e: ce})
				}
				continue
			}
			out, keys = append(out, top.e), append(keys, top.key)
			stack = stack[:len(stack)-1]
		}
	}
	return out, keys, stopped
}

// dropSliced forgets the kept blocks with keys, dropSlice at a time.
func (s *PendingStore) dropSliced(keys []string) {
	for len(keys) != 0 {
		n := min(len(keys), dropSlice)
		s.drop(keys[:n])
		keys = keys[n:]
	}
}

// unflushed returns the entries of tree, with keys, no open Flush stored.
// When it leaves one out, it counts the caller's SetRoots in flight until the
// returned landed is called, so no flush releases the stage that holds the
// block before the SetRoots that names it lands.
func (s *PendingStore) unflushed(tree []*PutBatchEntry, keys []string) (out []*PutBatchEntry, landed func()) {
	// Return the tree whole while nothing is flushed.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.flushed) == 0 {
		return tree, func() {}
	}

	// Leave out the flushed blocks.
	out = make([]*PutBatchEntry, 0, len(tree))
	for i, e := range tree {
		if s.flushed[keys[i]] == 0 {
			out = append(out, e)
		}
	}
	if len(out) == len(tree) {
		return out, func() {}
	}

	// Hold the flushes' stages until the SetRoots lands.
	s.skipping.Add(1)
	var once sync.Once
	return out, func() { once.Do(s.landed) }
}

// landed ends one SetRoots or Flush that left out a flushed block, and runs
// the stage releases that waited for the last of them.
func (s *PendingStore) landed() {
	// Take the waiting releases when the last such SetRoots lands.
	s.mtx.Lock()
	var run []func()
	if s.skipping.Add(-1) == 0 {
		run, s.deferred = s.deferred, nil
	}
	s.mtx.Unlock()

	// Run them.
	for _, fn := range run {
		fn()
	}
}

// afterSkips runs release once no SetRoots or Flush that left out a flushed
// block is open, at once when none is.
func (s *PendingStore) afterSkips(release func()) {
	// Defer it while such a SetRoots is in flight.
	s.mtx.Lock()
	if s.skipping.Load() != 0 {
		s.deferred = append(s.deferred, release)
		s.mtx.Unlock()
		return
	}
	s.mtx.Unlock()

	// Release now otherwise.
	release()
}
