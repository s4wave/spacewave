package s4db

import (
	"sync/atomic"
)

// refStripes is the number of counters a state's snapshot count spreads
// over, so concurrent readers do not share one cache line.
const refStripes = 16

// refCount counts a state's snapshots in stripes. A state is read while any
// stripe is nonzero.
type refCount [refStripes]struct {
	// n is this stripe's count.
	n atomic.Int64
	// _ pads the stripe to its own cache line.
	_ [56]byte
}

// held reports whether any snapshot reads the state.
func (r *refCount) held() bool {
	for i := range r {
		if r[i].n.Load() != 0 {
			return true
		}
	}
	return false
}
