package block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
)

// PublicationReceipt distinguishes admission from completion. Err is meaningful
// only after Done closes. Wait cancellation means the outcome is still unknown;
// the receipt remains usable for obtaining the eventual durable result.
type PublicationReceipt struct {
	// doneCh closes exactly once when the owning publisher resolves the receipt.
	doneCh chan struct{}
	// bcast guards the resolved and err state below.
	bcast    broadcast.Broadcast
	resolved bool
	// err is the durable result, meaningful only after resolved.
	err error
	// release drops optional publication ownership after durable completion.
	release     func()
	releaseOnce sync.Once
}

// NewPublicationReceipt constructs an unresolved completion owned by a writer.
func NewPublicationReceipt() *PublicationReceipt {
	return &PublicationReceipt{doneCh: make(chan struct{})}
}

// NewRetainedPublicationReceipt retains a prepared root until Release. Its
// owner supplies an idempotent cleanup that is valid after either outcome.
func NewRetainedPublicationReceipt(release func()) *PublicationReceipt {
	r := NewPublicationReceipt()
	r.release = release
	return r
}

// Release waits for resolution and releases publication ownership once. A
// consumer must first install its own reader pin if it will keep using the root.
func (r *PublicationReceipt) Release() {
	<-r.doneCh
	r.releaseOnce.Do(func() {
		if r.release != nil {
			r.release()
		}
	})
}

// Done returns a channel that closes when the owning publisher resolves the
// receipt.
func (r *PublicationReceipt) Done() <-chan struct{} { return r.doneCh }

// Resolve completes a receipt exactly once. Only its owning publisher calls it.
func (r *PublicationReceipt) Resolve(err error) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.resolved {
			return
		}
		r.resolved = true
		r.err = err
		close(r.doneCh)
		broadcast()
	})
}

// Wait returns the durable result, blocking until resolution when unresolved.
func (r *PublicationReceipt) Wait(ctx context.Context) error {
	var waitCh <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if r.resolved {
			return
		}
		waitCh = getWaitCh()
	})
	if waitCh == nil {
		return r.err
	}
	select {
	case <-waitCh:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
