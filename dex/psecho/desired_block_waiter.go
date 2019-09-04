package psecho

import (
	"github.com/aperturerobotics/hydra/cid"
)

// desiredBlockWaiter contains waiting state.
type desiredBlockWaiter struct {
	// ref is the desired block reference
	ref *cid.BlockRef
	// xmit indicates if the want has been transmitted yet
	// private to the execute routine
	xmit bool
	// refcount is the number of waiting routines
	// guarded by parent mtx
	refcount int

	// doneCh is closed when the waiter is done.
	// closed by the execute() routine
	doneCh chan struct{}
	// data contains the fetched block data
	data []byte
	// err contains any error received
	err error
}

// newDesiredBlockWaiter constructs a new desired block waiter.
func newDesiredBlockWaiter(ref *cid.BlockRef) *desiredBlockWaiter {
	return &desiredBlockWaiter{
		ref:    ref,
		doneCh: make(chan struct{}),
	}
}
