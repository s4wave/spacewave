package world_block

import (
	"sync/atomic"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// heldHead is a cursor on a head and the hold that keeps its root pinned. The
// snapshot owning it holds one reference and every object read holds another,
// so a detach releasing the snapshot's reference never releases the cursor
// under a read cloning it.
type heldHead struct {
	// refs counts the holders. Zero means released, and a released head is
	// never held again.
	refs atomic.Int64
	// root is the held cursor; only cloned, never changed.
	root *bucket_lookup.Cursor
	// free drops root and the pin after the last holder.
	free func()
}

// newHeldHead holds root for its first holder until free.
func newHeldHead(root *bucket_lookup.Cursor, free func()) *heldHead {
	h := &heldHead{root: root, free: free}
	h.refs.Store(1)
	return h
}

// tryHold adds a holder unless the head was released.
func (h *heldHead) tryHold() bool {
	// Count up only from a live count, so a released head stays released.
	for {
		n := h.refs.Load()
		if n <= 0 {
			return false
		}
		if h.refs.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// release drops a holder and frees the head after the last one.
func (h *heldHead) release() {
	if h.refs.Add(-1) == 0 {
		h.free()
	}
}
