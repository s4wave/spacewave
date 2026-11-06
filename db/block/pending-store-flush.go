package block

import (
	"context"
	"errors"
	"sync"
)

// flushSliceBytes bounds the data one put of a Flush writes, so a writer
// waiting on the inner store's writer waits for at most one slice.
const flushSliceBytes = 4 << 20

// Flush stores the kept trees the roots reach under a stage of the inner
// store, in puts of at most flushSliceBytes, and pins each root it keeps no
// block of and the inner store lacks, which brings a root another store backs,
// such as a peer, into the inner store; a root no store holds is left to the
// swap. Until release, SetRoots puts no block
// the flush stored, so a writer that flushes before it takes the lock its root
// moves need holds that lock, and the inner store's writer, only for the swap:
// the stage holds the stored blocks until the swap names them. The flushed
// blocks stay kept, so the Worlds that name them still read them from memory,
// and leave memory with the SetRoots that names them. Release after that
// SetRoots returns.
//
// Cost: one inner put per slice of the kept trees, and one existence read and
// pin per unkept root, all before the caller's lock.
func (s *PendingStore) Flush(ctx context.Context, roots []*BlockRef) (release func(), err error) {
	// Gather the kept trees and the roots kept nowhere here.
	s.mtx.Lock()
	tree, keys := s.reachLocked(roots)
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
	cleanup := func() {
		for _, unpin := range pins {
			unpin()
		}
		releaseStage()
	}

	// Store the trees in slices, children before parents.
	var batch []*PutBatchEntry
	var batchBytes int
	for i, e := range tree {
		batch, batchBytes = append(batch, e), batchBytes+len(e.Data)
		if batchBytes < flushSliceBytes && i != len(tree)-1 {
			continue
		}
		if _, err := stage.PutBlockBatch(ctx, batch); err != nil {
			cleanup()
			return nil, err
		}
		batch, batchBytes = nil, 0
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

	// Mark the stored blocks until release.
	s.mtx.Lock()
	for _, key := range keys {
		s.flushed[key]++
	}
	s.mtx.Unlock()

	// Unmark them and release the stage once.
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mtx.Lock()
			for _, key := range keys {
				if s.flushed[key]--; s.flushed[key] <= 0 {
					delete(s.flushed, key)
				}
			}
			s.mtx.Unlock()
			cleanup()
		})
	}, nil
}

// unflushed returns the entries of tree, with keys, no open Flush stored.
func (s *PendingStore) unflushed(tree []*PutBatchEntry, keys []string) []*PutBatchEntry {
	// Return the tree whole while nothing is flushed.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.flushed) == 0 {
		return tree
	}

	// Leave out the flushed blocks.
	out := make([]*PutBatchEntry, 0, len(tree))
	for i, e := range tree {
		if s.flushed[keys[i]] == 0 {
			out = append(out, e)
		}
	}
	return out
}
