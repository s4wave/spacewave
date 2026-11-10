package packahead

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// Evict unlinks finalized pack seq and forgets its values, so reads of them
// go to the remote. The caller must have proven that the remote holds the
// pack's object and entry and that the pack index registers it (EvictCheck
// and EvictedRegistered in PackAhead.tla): this is the only call that
// deletes a value the Log acknowledged. A value written again into a later
// pack stays readable from that pack. Evicting a pack the Log does not hold
// as finalized does nothing.
func (l *Log) Evict(ctx context.Context, seq uint64) error {
	// Find the finalized pack.
	var p *pack
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, q := range l.packs {
			if q.seq == seq && q.state == packFinal {
				p = q
				break
			}
		}
	})
	if p == nil {
		return nil
	}

	// Read the pack's keys from its index outside the lock.
	index, err := packfile.ReadIndex(p.f, uint64(p.written)) //nolint:gosec // non-negative.
	if err != nil {
		return errors.Wrapf(err, "read pack %s index", seqText(seq))
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Forget the values this pack still locates, then drop the pack. A Get
	// that located a value before this sees the pack gone and misses.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, e := range index {
			tail := tailOf(e.GetKey())
			if loc, ok := l.table[tail]; ok && loc.p == p {
				delete(l.table, tail)
			}
		}
		l.packs = slices.DeleteFunc(l.packs, func(q *pack) bool { return q == p })
		p.state = packGone
		broadcast()
	})

	// Unlink the file and make the unlink durable. A crash before the
	// directory sync brings the pack back at Open, which only uploads it
	// again.
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) { // #nosec G703 -- a pack in the Log's directory.
		return errors.Wrapf(err, "unlink pack %s", seqText(seq))
	}
	if err := syncDir(l.dir); err != nil {
		return errors.Wrap(err, "sync pack directory")
	}
	_ = p.f.Close()
	return nil
}

// MarkUploaded records that the Log's remote holds finalized pack seq, so a
// Drain may evict it. A Log drains to one remote, so the mark
// means that remote.
func (l *Log) MarkUploaded(seq uint64) {
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, p := range l.packs {
			if p.seq == seq && p.state == packFinal && !p.uploaded {
				p.uploaded = true
				l.dropRemovedLocked(p)
				broadcast()
				return
			}
		}
	})
}

// dropRemovedLocked forgets the values of uploaded pack p that Remove
// marked, which the remote now serves. The caller holds bcast.
func (l *Log) dropRemovedLocked(p *pack) {
	if p.removed <= 0 {
		return
	}
	for t, loc := range l.table {
		if loc.p == p && loc.removed {
			delete(l.table, t)
		}
	}
	p.removed = 0
}

// HoldsBefore reports whether a pack not marked uploaded holds a value whose
// batch was written before at. A pack loaded at Open has no write time and
// counts as written before any at.
func (l *Log) HoldsBefore(at time.Time) bool {
	var held bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, p := range append(slices.Clone(l.packs), l.open) {
			if p == nil || p.state == packGone || p.uploaded || (p.state != packFinal && len(p.entries) == 0) {
				continue
			}
			if !p.firstAt.IsZero() && !p.firstAt.Before(at) {
				continue
			}
			held = true
			return
		}
	})
	return held
}
