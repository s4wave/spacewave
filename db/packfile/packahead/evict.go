package packahead

import (
	"cmp"
	"context"
	"os"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
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
	p, index, err := l.evicting(ctx, seq)
	if err != nil || p == nil {
		return err
	}
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
	// Find the pack and read its index.
	p, index, err := l.evicting(ctx, seq)
	if err != nil || p == nil {
		return 0, err
	}

	// Choose the read values the pack still locates, in offset order.
	type carried struct {
		key []byte
		loc location
	}
	var carry []carried
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, e := range index {
			loc, ok := l.table[tailOf(e.GetKey())]
			if ok && loc.p == p && loc.read && int64(loc.size) <= carryBytes {
				carryBytes -= int64(loc.size)
				carry = append(carry, carried{key: e.GetKey(), loc: loc})
			}
		}
	})
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

// evicting returns finalized pack seq and its index, read outside the lock,
// or a nil pack when the Log holds no finalized pack seq.
func (l *Log) evicting(ctx context.Context, seq uint64) (*pack, []*kvfile.IndexEntry, error) {
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
		return nil, nil, nil
	}

	// Read the pack's keys from its index outside the lock.
	index, err := packfile.ReadIndex(p.f, uint64(p.written)) //nolint:gosec // non-negative.
	if err != nil {
		return nil, nil, errors.Wrapf(err, "read pack %s index", seqText(seq))
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return p, index, nil
}

// unlink forgets the values finalized pack p still locates, drops it, and
// unlinks its file.
func (l *Log) unlink(p *pack, index []*kvfile.IndexEntry) error {
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
		return errors.Wrapf(err, "unlink pack %s", seqText(p.seq))
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
