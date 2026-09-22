package block

import (
	"context"
	"slices"
)

// SyncReachable discards pending writes outside roots, then applies the normal
// durability fence. Reachability follows the outgoing refs recorded with each
// pending block; references already in the inner store are retained there.
//
// The caller must exclusively own the buffer and stop all writers first. Use
// this for a newly constructed snapshot, not a buffer shared with other work.
// It never removes blocks from the inner store.
func (s *BufferedStore) SyncReachable(ctx context.Context, roots ...*BlockRef) (bool, error) {
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return false, err
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		keep := make(map[string]struct{})
		type visit struct {
			ref *BlockRef
			key string
		}
		stack := make([]visit, 0, len(roots))
		for _, root := range roots {
			stack = append(stack, visit{ref: root})
		}
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
	if err != nil {
		return false, err
	}
	return s.Sync(ctx)
}
