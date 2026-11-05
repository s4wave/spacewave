package world_block

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/sirupsen/logrus"
)

// defaultReaderPinDelay is how long a replaced head may stay held by a reader
// before the engine pins its root. While a head is current, the authority's
// named root holds it; once replaced, the collector's grace period keeps its
// blocks for a reader that has not pinned it. The delay is well inside the
// shortest production grace, which is longer than the five minute root lease,
// so a short reader costs no Volume transaction and a long one costs one.
const defaultReaderPinDelay = time.Minute

// publishedHead is an immutable published head that readers load and hold
// without the Engine lock. The Engine holds one reference while the head is
// published and every reader holds another until it releases, so a reader
// keeps reading the storage of the head it pinned after a newer head
// replaces it.
type publishedHead struct {
	// refs counts the holders. Zero means retired, and a retired head is
	// never held again.
	refs atomic.Int64
	// root is a cursor on the head root, only cloned, never changed.
	root *bucket_lookup.Cursor
	// ref is the head root reference, never changed.
	ref *bucket.ObjectRef
	// rootBlock is the decoded World root, or nil. Callers clone it before use.
	rootBlock *World
	// seqno is the revision of the head.
	seqno uint64
	// pinMtx guards unpin and pinTimer.
	pinMtx sync.Mutex
	// unpin releases the reader pin taken after the head was replaced, or is
	// nil while no pin is held.
	unpin func()
	// pinTimer pins the root once the head stayed held past the pin delay
	// after it was replaced, or is nil.
	pinTimer *time.Timer
}

// newPublishedHead returns a head at root holding one reference for the
// Engine.
func newPublishedHead(root *bucket_lookup.Cursor, rootBlock *World, seqno uint64) *publishedHead {
	p := &publishedHead{
		root:      root.Clone(),
		ref:       root.GetRef().Clone(),
		rootBlock: rootBlock,
		seqno:     seqno,
	}
	p.refs.Store(1)
	return p
}

// tryHold adds a holder unless the head has retired.
func (p *publishedHead) tryHold() bool {
	// Count up only from a live count, so a retired head stays retired.
	for {
		n := p.refs.Load()
		if n <= 0 {
			return false
		}
		if p.refs.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// hold adds a holder. The caller must already hold the head.
func (p *publishedHead) hold() func() {
	p.refs.Add(1)
	return p.release
}

// release drops a holder and releases the root and any pin after the last one.
func (p *publishedHead) release() {
	// Keep the head while another holder remains.
	if p.refs.Add(-1) != 0 {
		return
	}

	// Stop a pending pin and drop a held one.
	p.pinMtx.Lock()
	timer, unpin := p.pinTimer, p.unpin
	p.pinTimer, p.unpin = nil, nil
	p.pinMtx.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if unpin != nil {
		unpin()
	}
	p.root.Release()
}

// retire drops the Engine's reference to a replaced head. When readers still
// hold it, the root is pinned in store if they still do after delay. A delay
// of zero pins at once, before the commit that replaced the head returns.
func (p *publishedHead) retire(le *logrus.Entry, store block.StoreOps, delay time.Duration) {
	// Pin at once when the delay is zero.
	if delay == 0 && p.refs.Load() > 1 {
		p.pinHeld(le, store, delay)
		p.release()
		return
	}

	// Arm the pin before dropping the Engine's reference, so a reader that
	// releases meanwhile stops it.
	if p.refs.Load() > 1 {
		p.armPin(le, store, delay)
	}
	p.release()
}

// armPin pins the root after delay if a reader still holds the head.
func (p *publishedHead) armPin(le *logrus.Entry, store block.StoreOps, delay time.Duration) {
	p.pinMtx.Lock()
	p.pinTimer = time.AfterFunc(delay, func() { p.pinHeld(le, store, delay) })
	p.pinMtx.Unlock()
}

// pinHeld pins the root while a reader still holds the head. The pin holds a
// reference of its own while it writes, so the last release sees the unpin. A
// pin that fails is tried again after delay, while a reader holds the head.
func (p *publishedHead) pinHeld(le *logrus.Entry, store block.StoreOps, delay time.Duration) {
	// Skip a head every reader released.
	if !p.tryHold() {
		return
	}
	defer p.release()

	// Pin the root and keep the pin until the last release, or try again.
	unpin, err := block.PinRoot(context.Background(), store, p.ref.GetRootRef())
	if err != nil {
		le.WithError(err).Warn("unable to pin a replaced World head a reader holds")
		if delay != 0 {
			p.armPin(le, store, delay)
		}
		return
	}
	p.pinMtx.Lock()
	p.unpin, p.pinTimer = unpin, nil
	p.pinMtx.Unlock()
}
