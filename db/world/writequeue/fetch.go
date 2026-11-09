package writequeue

import (
	"slices"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// peerFetchRounds bounds the times one write leaves its batch to fetch blocks
// from peers. A write past it reads from peers under the writer lock, as every
// write did before writes fetched outside it. A write that parked again past
// it would never commit, and neither would the writes requeued with it.
const peerFetchRounds = 8

// A write body runs under the engine's writer lock, so a read that only a
// peer can serve would hold the lock, and every write queued behind it, for as
// long as the peer takes to answer. A body therefore runs under
// block.WithPeerReads: such a read fails at once and records the block. The
// queue then drops the batch's transaction, sets the write aside while it
// fetches the blocks with no lock held, and queues it again; the writes of
// other actors go on meanwhile, and the later writes of its actor wait for it,
// so one actor's writes keep their order. Moving a write behind later writes
// of other actors is safe: its writer has had no answer, so no caller can have
// seen its change. A write already answered at an admission the engine then
// dropped may have been seen, so it reads from peers under the lock instead.

// parkedLocked reports whether w waits behind a write of its actor that is
// fetching, with mtx held.
func (q *Queue) parkedLocked(w *queuedWrite) bool {
	return w.actor != 0 && q.fetching[w.actor] != 0
}

// runnableLocked reports whether a pending write may join a batch, with mtx
// held.
func (q *Queue) runnableLocked() bool {
	return slices.ContainsFunc(q.pending, func(w *queuedWrite) bool { return !q.parkedLocked(w) })
}

// park sets w aside to fetch the blocks its body deferred, queueing the rest
// of its batch again. Only an attempt under peerReads parks, so a write parks
// at most peerFetchRounds times.
func (q *Queue) park(w *queuedWrite, rest []*queuedWrite) {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	if w.actor != 0 {
		if q.fetching == nil {
			q.fetching = make(map[uint64]int)
		}
		q.fetching[w.actor]++
	}
	q.parked = append(q.parked, w)
	q.requeueLocked(rest)
	go q.fetch(w)
}

// fetch reads the blocks w deferred, with no lock held, and queues w again.
// A block no peer served leaves w to read from peers under the lock, as it
// would have before, so the write's result is the read's.
func (q *Queue) fetch(w *queuedWrite) {
	started := time.Now()
	ok, err := w.peerReads.Fetch(block.WithReadClass(w.ctx, block.ReadUrgent))
	le := logrus.WithFields(logrus.Fields{
		"op":      w.name(),
		"actor":   w.actor,
		"round":   w.peerReads.Rounds(),
		"elapsed": time.Since(started).Round(time.Millisecond).String(),
	})
	if !ok {
		le.WithError(err).Warn("World write's peer fetch missed; it reads from peers under the writer lock")
		w.peerWait = true
	} else {
		le.Info("World write fetched blocks from peers outside the writer lock")
	}

	q.mtx.Lock()
	defer q.mtx.Unlock()
	if w.actor != 0 {
		if q.fetching[w.actor]--; q.fetching[w.actor] == 0 {
			delete(q.fetching, w.actor)
		}
	}
	q.parked = slices.DeleteFunc(q.parked, func(p *queuedWrite) bool { return p == w })
	q.requeueLocked([]*queuedWrite{w})
}
