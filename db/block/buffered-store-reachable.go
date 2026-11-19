package block

import (
	"context"
	fmt "fmt"
	"slices"
)

// SyncReachable discards pending writes outside roots, then applies the normal
// durability fence. Reachability follows the outgoing refs of each pending
// block and, when the store records writes, of each block it already wrote to
// the inner store. A capacity drain can write a parent before its children
// arrive, so a store that may drain before this call must record writes.
//
// The caller must exclusively own the buffer and stop all writers first. Use
// this for a newly constructed snapshot, not a buffer shared with other work.
// It never removes blocks from the inner store.
func (s *BufferedStore) SyncReachable(ctx context.Context, roots ...*BlockRef) (bool, error) {
	if _, err := s.keepReachable(ctx, roots); err != nil {
		return false, err
	}
	return s.Sync(ctx)
}

// DrainReachable discards pending writes outside roots, then writes the rest
// to the inner store. It neither drains inner buffered layers nor applies a
// durability fence. Ownership is as for SyncReachable.
func (s *BufferedStore) DrainReachable(ctx context.Context, roots ...*BlockRef) error {
	if _, err := s.keepReachable(ctx, roots); err != nil {
		return err
	}
	return s.drainAll(ctx)
}

// TakeReachable discards pending writes outside roots and borrows the rest,
// children before parents, without writing them, so a caller can store them
// in one transaction of its own. Ownership is as for SyncReachable, and the
// borrow is as for TakePending.
//
// A pending block that a capacity drain's parent names is written to the
// inner store first, with every pending block below it, and not borrowed: the
// inner store holds the parent, so it holds them only when they reach it the
// same way. Borrowed, they would land beside a root move that holds them only
// through the drained parent's list, which the inner store cannot see in the
// batch. Only a commit too large for the buffer pays this write.
func (s *BufferedStore) TakeReachable(ctx context.Context, roots ...*BlockRef) (*PendingBatch, error) {
	under, err := s.keepReachable(ctx, roots)
	if err != nil {
		return nil, err
	}
	if under != 0 {
		if err := s.drainQueuePrefix(ctx, under); err != nil {
			return nil, err
		}
	}
	return s.TakePending(ctx)
}

// drainQueuePrefix writes the first n queued blocks to the inner store,
// leaving the rest queued.
func (s *BufferedStore) drainQueuePrefix(ctx context.Context, n int) error {
	// Take the drain lock, so no other drain writes meanwhile.
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Set the rest aside, drain the prefix, and queue the rest ahead of any
	// block put meanwhile, so it keeps its children-first order.
	var rest []string
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rest = slices.Clone(s.queue[n:])
		s.queue = s.queue[:n]
	})
	err = s.drainQueue(ctx)
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.queue = append(rest, s.queue...)
	})
	return err
}

// keepReachable drops every pending block that neither roots nor a reader pin
// reach and queues the rest with children before parents. The queue starts
// with the under blocks it reports, those first reached through a block the
// inner store already holds, so a caller can write them before the rest.
func (s *BufferedStore) keepReachable(ctx context.Context, roots []*BlockRef) (int, error) {
	// Hold the drain lock for the exclusive reachable-set rewrite.
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return 0, err
	}
	var under []string
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Walk the pending graph from the roots, ordering children first,
		// and mark each visit reached through a written block.
		keep := make(map[string]struct{})
		type visit struct {
			ref   *BlockRef
			key   string
			under bool
		}

		// Start from the roots and every pinned root.
		stack := make([]visit, 0, len(roots)+len(s.pins))
		for _, root := range roots {
			stack = append(stack, visit{ref: root})
		}
		for _, pins := range s.pins {
			stack = append(stack, visit{ref: pins[0].ref})
		}

		// Pop each visit, retaining reachable pending blocks in order.
		var queue []string
		for len(stack) != 0 {
			if err = ctx.Err(); err != nil {
				return
			}
			next := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if next.key != "" {
				if next.under {
					under = append(under, next.key)
				} else {
					queue = append(queue, next.key)
				}
				continue
			}
			ref := next.ref
			if ref.GetEmpty() {
				continue
			}
			var key string
			key, err = marshalRefKey(ref)
			if err != nil {
				return
			}
			if _, seen := keep[key]; seen {
				continue
			}
			keep[key] = struct{}{}
			pending := s.pending[key]
			if pending == nil {
				// Walk through a block a capacity drain already wrote, so its
				// pending children survive.
				if written := s.written[key]; written != nil {
					for _, childRef := range slices.Backward(written.refs) {
						stack = append(stack, visit{ref: childRef, under: true})
					}
				}
				continue
			}
			if pending.tombstone {
				err = fmt.Errorf("pending tombstone under the committed root: %w", NotFoundError(ref))
				return
			}
			// Emit children immediately before their parent. Keeping a DAG
			// neighborhood in one storage batch avoids temporary ownership
			// edges that a later batch would immediately remove.
			stack = append(stack, visit{key: key, under: next.under})
			for _, childRef := range slices.Backward(pending.refs) {
				stack = append(stack, visit{ref: childRef, under: next.under})
			}
		}
		s.queue = append(under, queue...)

		// Drop every pending block outside the retained set.
		for key, pending := range s.pending {
			if _, retained := keep[key]; !retained {
				delete(s.pending, key)
				s.pendingBytes -= len(pending.data)
				s.pendingMetadataBytes -= pending.metadataBytes
			}
		}
		broadcast()
	})
	release()
	return len(under), err
}
