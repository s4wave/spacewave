package block

import (
	"context"
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
	if err := s.keepReachable(ctx, roots); err != nil {
		return false, err
	}
	return s.Sync(ctx)
}

// DrainReachable discards pending writes outside roots, then writes the rest
// to the inner store. It neither drains inner buffered layers nor applies a
// durability fence. Ownership is as for SyncReachable.
func (s *BufferedStore) DrainReachable(ctx context.Context, roots ...*BlockRef) error {
	if err := s.keepReachable(ctx, roots); err != nil {
		return err
	}
	return s.drainAll(ctx)
}

// keepReachable drops every pending block that neither roots nor a reader pin
// reach and queues the rest with children before parents.
func (s *BufferedStore) keepReachable(ctx context.Context, roots []*BlockRef) error {
	// Hold the drain lock for the exclusive reachable-set rewrite.
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return err
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Walk the pending graph from the roots, ordering children first.
		keep := make(map[string]struct{})
		type visit struct {
			ref *BlockRef
			key string
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
				queue = append(queue, next.key)
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
						stack = append(stack, visit{ref: childRef})
					}
				}
				continue
			}
			if pending.tombstone {
				err = ErrNotFound
				return
			}
			// Emit children immediately before their parent. Keeping a DAG
			// neighborhood in one storage batch avoids temporary ownership
			// edges that a later batch would immediately remove.
			stack = append(stack, visit{key: key})
			for _, childRef := range slices.Backward(pending.refs) {
				stack = append(stack, visit{ref: childRef})
			}
		}
		s.queue = queue

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
	return err
}
