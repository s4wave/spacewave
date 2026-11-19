package packahead

import (
	"container/list"
	"context"
	"os"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// Evict unlinks finalized pack seq and forgets its values, so reads of them
// go to the remote. The caller must have proven that the remote holds the
// pack's object and entry and that the pack index registers it (EvictCheck
// and EvictedRegistered in PackAhead.tla). A value written again into a
// later pack stays readable there. Evicting an unknown pack does nothing.
func (l *Log) Evict(ctx context.Context, seq uint64) error {
	// Claim the finalized pack. Two evictions of the same pack share the
	// outcome through the registry and its watch.
	var p *pack
	var uploaded bool
	if err := l.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		// Close stops new claims and waits for claims already in flight.
		if l.closed {
			return true, ErrClosed
		}
		p = l.packs[seq]
		if p == nil || p.state != packFinal {
			p = nil
			return true, nil
		}
		if p.evicting {
			return false, nil
		}
		p.evicting = true
		uploaded = p.uploaded
		l.evictions++
		return true, nil
	}); err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	defer l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		p.evicting = false
		l.evictions--
		broadcast()
	})

	// Read the keys outside the lock and allow cancellation before unlinking.
	index, err := packfile.ReadIndex(p.f, uint64(p.written)) //nolint:gosec // non-negative.
	if err != nil {
		return errors.Wrapf(err, "read pack %s index", seqText(seq))
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// The caller proved the remote durably holds the pack. Apply deferred
	// removals before losing the table entries that identify their backing.
	if !uploaded {
		l.MarkUploaded(seq)
	}

	// Unlink while keeping the descriptor readable. If either disk step
	// fails, the descriptor and registry stay available for another attempt.
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) { // #nosec G703 -- a pack in the Log's directory.
		return errors.Wrapf(err, "unlink pack %s", seqText(seq))
	}
	if err := dirSync(l.dir); err != nil {
		return errors.Wrap(err, "sync pack directory")
	}

	// I4: table cleanup holds bcast for one fixed-size batch at a time.
	// The remote copy remains readable, so these local drops need no fence.
	for batch := range slices.Chunk(index, locationBatchSize) {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, e := range batch {
				tail := tailOf(e.GetKey())
				if loc, ok := l.table[tail]; ok && loc.p == p {
					delete(l.table, tail)
				}
			}
		})
	}

	// Release the descriptor after publishing the loss of the local copy.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Withdraw the evicted pack from the finalized view.
		delete(l.packs, seq)
		l.finals.Remove(p.order)
		l.localBytes -= p.written
		l.finalBytes -= p.written
		l.finalVersion++
		p.state = packGone
	})
	_ = p.f.Close()
	return nil
}

// MarkUploaded records that the Log's remote holds finalized pack seq. A Log
// drains to one remote, so the mark means that remote.
func (l *Log) MarkUploaded(seq uint64) {
	for {
		// I4: an indexed lookup and at most one batch of deferred removals
		// hold bcast. A concurrent put can still revive a queued location.
		var more bool
		l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Recheck the upload and process its next batch of locations.
			p := l.packs[seq]
			if p == nil || p.state != packFinal || p.uploaded {
				return
			}
			l.dropRemovedLocked(p)
			if more = p.removals.Len() != 0; more {
				return
			}

			// Finish the upload only once every deferred drop is visible.
			p.uploaded = true
			l.pending.Remove(p.pending)
			l.held.Remove(p.held)
			p.pending, p.held = nil, nil
			l.pendingBytes -= p.written
			l.finalVersion++
			broadcast()
		})
		if !more {
			return
		}
	}
}

// deferredRemoval is a location to forget once the pack has a remote copy.
type deferredRemoval struct {
	// tail names the location in the log's table.
	tail keyTail
	// stripe is the decoded read fence for that location.
	stripe block.BlockRemovalStripe
}

// deferRemoval queues the location under tail once. The caller holds bcast.
func (p *pack) deferRemoval(tail keyTail, stripe block.BlockRemovalStripe) {
	// Index the queue so a later put can revive the location in fixed work.
	if p.removed == nil {
		p.removed = make(map[keyTail]*list.Element)
	}
	if p.removed[tail] == nil {
		p.removed[tail] = p.removals.PushBack(deferredRemoval{tail: tail, stripe: stripe})
	}
}

// revive cancels a deferred removal. The caller holds bcast.
func (p *pack) revive(tail keyTail) {
	// Remove the same record from its index and queue.
	if e := p.removed[tail]; e != nil {
		delete(p.removed, tail)
		p.removals.Remove(e)
	}
}

// dropRemovedLocked forgets one batch of deferred locations in a pack
// whose remote copy is now available. The caller holds bcast.
func (l *Log) dropRemovedLocked(p *pack) {
	// The common case has no deferred locations to fence.
	if p.removals.Len() == 0 {
		return
	}

	// Remove only locations that still reference this pack.
	var removals block.BlockRemovalBatch
	for range min(locationBatchSize, p.removals.Len()) {
		e := p.removals.Front().Value.(deferredRemoval)
		if loc, ok := l.table[e.tail]; ok && loc.p == p {
			delete(l.table, e.tail)
			removals.Add(e.stripe)
		}
		p.revive(e.tail)
	}

	// I2: publish the generation before log readers can observe the drops.
	removals.Publish()
}

// HoldsBefore reports whether a pack not marked uploaded holds a value whose
// batch was written before at. A pack loaded at Open has no write time and
// counts as written before any at.
func (l *Log) HoldsBefore(at time.Time) bool {
	var held bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if oldest := l.held.Front(); oldest != nil {
			first := oldest.Value.(*pack).firstAt
			held = first.IsZero() || first.Before(at)
		}
		held = held || (len(l.open.entries) != 0 && l.open.firstAt.Before(at))
	})
	return held
}
