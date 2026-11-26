package dex_solicit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// readProgressKey is the context key of a read's progress.
type readProgressKey struct{}

// readProgress counts the bytes delivered toward one read: the bytes each
// peer session read from its stream while one of the read's requests was
// pending on it. A session writes its answers in order, so the bytes of
// answers queued ahead of the read's own count too. A read that a peer
// answered with a failure counts nothing more, since its retries draw the
// same answer again; a closed session does not end the count, so the read's
// retry on the session that replaces it counts.
type readProgress struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// done is the bytes of the request windows that ended.
	done uint64
	// open are the request windows still pending.
	open map[*readWindow]struct{}
	// refused is set once a peer answered the read with a failure, and
	// frozen holds the count at that moment.
	refused bool
	frozen  uint64
}

// readWindow is one request of a read pending on a session.
type readWindow struct {
	// received is the session's stream byte count.
	received *atomic.Uint64
	// start is received when the request went pending.
	start uint64
}

// begin opens a window on a session's received count when a request of the
// read goes pending there.
func (p *readProgress) begin(received *atomic.Uint64) *readWindow {
	// Mark the session's count, and add the window to the open set.
	w := &readWindow{received: received, start: received.Load()}
	p.mtx.Lock()
	if p.open == nil {
		p.open = make(map[*readWindow]struct{}, 1)
	}
	p.open[w] = struct{}{}
	p.mtx.Unlock()
	return w
}

// end closes a window once its request is answered or abandoned.
func (p *readProgress) end(w *readWindow) {
	p.mtx.Lock()
	delete(p.open, w)
	p.done += w.received.Load() - w.start
	p.mtx.Unlock()
}

// refuse freezes the count once a peer answered the read with a failure.
func (p *readProgress) refuse() {
	p.mtx.Lock()
	if !p.refused {
		p.frozen, p.refused = p.countLocked(), true
	}
	p.mtx.Unlock()
}

// bytes returns the bytes delivered toward the read so far.
func (p *readProgress) bytes() uint64 {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if p.refused {
		return p.frozen
	}
	return p.countLocked()
}

// countLocked sums the ended and the open windows. The caller holds mtx.
func (p *readProgress) countLocked() uint64 {
	n := p.done
	for w := range p.open {
		n += w.received.Load() - w.start
	}
	return n
}

// getReadProgress returns the progress ctx carries, or nil.
func getReadProgress(ctx context.Context) *readProgress {
	p, _ := ctx.Value(readProgressKey{}).(*readProgress)
	return p
}

// peerRefused reports whether err holds a peer's failure answer, as opposed
// to a closed session or the end of the read's own context.
func peerRefused(err error) bool {
	return errors.Is(err, ErrPeerError) ||
		errors.Is(err, ErrPeerDenied) ||
		errors.Is(err, ErrUnaskedLinked) ||
		errors.Is(err, block.ErrRefsUnknown)
}
