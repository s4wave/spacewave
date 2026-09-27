package engine

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/db/traceutil"
)

// snapshot pins immutable files across one read operation.
type snapshot struct {
	// engine supplies validated immutable file reads.
	engine *Engine
	// root fixes the operation's complete committed generation.
	root *Root
	// release relinquishes the cross-runtime reclamation lock.
	release func()
}

// snapshot acquires file protection before resolving the durable root.
func (e *Engine) snapshot(ctx context.Context) (*snapshot, error) {
	// Keep the selected generation's immutable files alive through the read.
	release, err := e.protect(ctx)
	if err != nil {
		return nil, err
	}

	// A retained browser File can become unreadable when its entry is replaced.
	// Copy both roots under a shared root lock, after reclamation protection.
	// Payload publication remains independent of this short critical section.
	unlock, err := e.backend.Lock(ctx, "root", false)
	if err != nil {
		release()
		return nil, err
	}
	defer unlock()
	root, err := e.readSnapshotRoot(ctx)
	if err != nil {
		release()
		return nil, err
	}
	return &snapshot{engine: e, root: root, release: release}, nil
}

// readSnapshotRoot shares overlapping descriptor reads. Every caller holds its
// own shared root lock, preventing publication while it joins or consumes the
// pending read. The result is removed before any of those locks are released;
// a later operation always rereads durable state.
func (e *Engine) readSnapshotRoot(ctx context.Context) (*Root, error) {
	for {
		// Join an existing read or perform it in the caller's own lifetime.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e.mtx.Lock()
		pending := e.rootRead
		leader := pending == nil
		if leader {
			pending = promise.NewPromise[*Root]()
			e.rootRead = pending
		}
		e.mtx.Unlock()
		if leader {
			traceutil.Log(ctx, "hydra/opfs-engine/root-read", "read")
			root, err := e.loadRoot(ctx)
			if ctx.Err() != nil {
				root, err = nil, ctx.Err()
			}
			e.mtx.Lock()
			e.rootRead = nil
			e.mtx.Unlock()
			pending.SetResult(root, err)
			return root, err
		}

		// One caller's cancellation cannot cancel another caller's snapshot.
		traceutil.Log(ctx, "hydra/opfs-engine/root-read", "joined")
		root, err := pending.Await(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		return root, err
	}
}

// get resolves a full key through one partition's bounded newest-first runs.
func (s *snapshot) get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Descend through catalogue bounds to the partition containing the key.
	name := s.root.Catalogue
	for {
		page, err := s.engine.readCatalogue(ctx, name)
		if err != nil {
			return nil, false, err
		}
		if len(page.Children) != 0 {
			i := sort.Search(len(page.Children), func(i int) bool { return bytes.Compare(page.Children[i].Lower, key) > 0 }) - 1
			if i < 0 {
				return nil, false, nil
			}
			name = page.Children[i].File
			continue
		}

		// Search the selected partition's runs from newest to oldest.
		i := sort.Search(len(page.Partitions), func(i int) bool { return bytes.Compare(page.Partitions[i].Lower, key) > 0 }) - 1
		if i < 0 {
			return nil, false, nil
		}
		runs := page.Partitions[i].Runs
		for _, file := range slices.Backward(runs) {
			run, err := s.engine.readRun(ctx, file.GetName())
			if err != nil {
				return nil, false, err
			}
			i := sort.Search(len(run.Records), func(i int) bool { return bytes.Compare(run.Records[i].Key, key) >= 0 })
			if i < len(run.Records) && bytes.Equal(run.Records[i].Key, key) {
				record := run.Records[i]
				return record.Value, !record.Deleted, nil
			}
		}
		return nil, false, nil
	}
}

// Get returns a caller-owned value and the generation that supplied it.
func (e *Engine) Get(ctx context.Context, key []byte) ([]byte, bool, uint64, error) {
	// Pin one durable generation for the complete lookup.
	s, err := e.snapshot(ctx)
	if err != nil {
		return nil, false, 0, err
	}
	defer s.release()

	// Return independent bytes with the revision that supplied them.
	value, found, err := s.get(ctx, key)
	return bytes.Clone(value), found, s.root.Revision, err
}
