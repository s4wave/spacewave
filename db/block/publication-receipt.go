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
	// existed reports, for each entry of a successful publication, whether
	// the store already held it, or is nil when the publisher did not say.
	existed []bool
	// counting reports whether the store's shared counts were complete when
	// it applied a successful publication.
	counting bool
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
	r.ResolveExisted(err, nil, false)
}

// ResolveExisted completes a receipt exactly once, reporting for each entry of
// the publication whether the store already held it, and whether the store's
// shared counts were complete when it applied the publication. Both are kept
// only when err is nil. Only its owning publisher calls it.
func (r *PublicationReceipt) ResolveExisted(err error, existed []bool, counting bool) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Resolve the receipt once and wake its waiters.
		if r.resolved {
			return
		}
		r.resolved = true
		r.err = err
		if err == nil {
			r.existed, r.counting = existed, counting
		}
		close(r.doneCh)
		broadcast()
	})
}

// Err returns the durable result. It is meaningful only after Done closes.
func (r *PublicationReceipt) Err() error {
	var err error
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = r.err
	})
	return err
}

// Existed reports, for each entry of the publication, whether the store
// already held it. It is meaningful only after Done closes with no error, and
// is nil when the publisher did not report it: the caller then treats every
// entry as one the store did not hold.
func (r *PublicationReceipt) Existed() []bool {
	var existed []bool
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		existed = r.existed
	})
	return existed
}

// Counting reports whether the store's shared counts were complete when it
// applied the publication, so its puts counted every holder they added as
// they wrote it rather than leaving it to a sharing walk. It is meaningful
// only after Done closes with no error, and is false when the publisher did
// not report it.
func (r *PublicationReceipt) Counting() bool {
	var counting bool
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		counting = r.counting
	})
	return counting
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
