package block

import (
	"context"

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
}

// NewPublicationReceipt constructs an unresolved completion owned by a writer.
func NewPublicationReceipt() *PublicationReceipt {
	return &PublicationReceipt{doneCh: make(chan struct{})}
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
