package packahead

import (
	"cmp"
	"container/list"
	"context"
	"os"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
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
	// Claim the pack and read its index.
	p, index, release, err := l.evicting(ctx, seq)
	if err != nil || p == nil {
		return err
	}
	defer release()

	// The caller proved the remote durably holds the pack. Apply deferred
	// removals before losing the table entries that identify their backing.
	l.MarkUploaded(seq)
	return l.unlink(p, index)
}

// carryBatchBytes bounds the values one carry append writes.
const carryBatchBytes = 1 << 20

// EvictCarry unlinks finalized pack seq as Evict does, after appending to the
// open pack, unsynced, the values Get read since their write into it, in
// offset order, up to carryBytes of them. A carried value is relocated in
// place, so a reader never misses it, and starts with its read bit clear, so
// it is carried again only if read again. It returns the values carried.
//
// It drops every value it does not carry, with no proof that a remote holds
// it, so only a Log of copies, such as a cache, calls it.
func (l *Log) EvictCarry(ctx context.Context, seq uint64, carryBytes int64) (int, error) {
	// Claim the pack and read its index.
	p, index, release, err := l.evicting(ctx, seq)
	if err != nil || p == nil {
		return 0, err
	}
	defer release()

	// Choose the read values the pack still locates and GC has not removed,
	// in offset order, one batch of index entries per lock hold.
	type carried struct {
		key []byte
		loc location
	}
	var carry []carried
	for batch := range slices.Chunk(index, locationBatchSize) {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, e := range batch {
				t := tailOf(e.GetKey())
				loc, ok := l.table[t]
				if ok && loc.p == p && loc.read && p.removed[t] == nil && int64(loc.size) <= carryBytes {
					carryBytes -= int64(loc.size)
					carry = append(carry, carried{key: e.GetKey(), loc: loc})
				}
			}
		})
	}
	slices.SortFunc(carry, func(a, b carried) int { return cmp.Compare(a.loc.off, b.loc.off) })

	// Read them in order and append them in batches, relocating each.
	var batch []Block
	var batchBytes int
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := l.write(ctx, batch, p)
		batch, batchBytes = batch[:0], 0
		return err
	}
	for _, c := range carry {
		value := make([]byte, c.loc.size)
		if err := readAtFull(p.f, value, int64(c.loc.off)); err != nil {
			return 0, errors.Wrapf(err, "read pack %s", seqText(seq))
		}
		batch = append(batch, Block{Key: c.key, Value: value})
		if batchBytes += len(value); batchBytes >= carryBatchBytes {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := flush(); err != nil {
		return 0, err
	}
	return len(carry), l.unlink(p, index)
}

// evicting claims finalized pack seq and reads its index outside the lock.
// It returns a nil pack when the Log holds no finalized pack seq; otherwise
// the caller calls release once it has finished with the pack. Two evictions
// of the same pack share the outcome through the registry and its watch.
func (l *Log) evicting(ctx context.Context, seq uint64) (*pack, []*kvfile.IndexEntry, func(), error) {
	// Claim the finalized pack.
	var p *pack
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
		l.evictions++
		return true, nil
	}); err != nil {
		return nil, nil, nil, err
	}
	if p == nil {
		return nil, nil, nil, nil
	}
	release := func() {
		l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			p.evicting = false
			l.evictions--
			broadcast()
		})
	}

	// Read the keys outside the lock and allow cancellation before unlinking.
	index, err := packfile.ReadIndex(p.f, uint64(p.written)) //nolint:gosec // non-negative.
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		release()
		return nil, nil, nil, errors.Wrapf(err, "read pack %s index", seqText(seq))
	}
	return p, index, release, nil
}

// unlink forgets the values claimed finalized pack p still locates, drops
// it, and unlinks its file.
func (l *Log) unlink(p *pack, index []*kvfile.IndexEntry) error {
	// Fence cached reads of blocks GC removed while the pack held them. Evict
	// has drained them already; a cache's pack drains them here.
	for more := true; more; {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			l.dropRemovedLocked(p)
			more = p.removals.Len() != 0
		})
	}

	// Unlink while keeping the descriptor readable. If either disk step
	// fails, the descriptor and registry stay available for another attempt.
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) { // #nosec G703 -- a pack in the Log's directory.
		return errors.Wrapf(err, "unlink pack %s", seqText(p.seq))
	}
	if err := dirSync(l.dir); err != nil {
		return errors.Wrap(err, "sync pack directory")
	}

	// I4: table cleanup holds bcast for one fixed-size batch at a time.
	// The values are content-addressed copies whose removals were fenced
	// above, so these local drops need no fence.
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
		delete(l.packs, p.seq)
		l.finals.Remove(p.order)
		l.localBytes -= p.written
		l.finalBytes -= p.written
		l.finalVersion++
		p.state = packGone

		// A pack never marked uploaded also leaves the upload and held queues.
		if p.pending != nil {
			l.pending.Remove(p.pending)
			l.pendingBytes -= p.written
		}
		if p.held != nil {
			l.held.Remove(p.held)
		}
		p.order, p.pending, p.held = nil, nil, nil
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
