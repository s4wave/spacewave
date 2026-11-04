package s4db

import (
	"context"

	"github.com/pkg/errors"
)

// sliceBuckets is the number of commit ranges chooseCut sums the overlay
// into.
const sliceBuckets = 4096

// logCut is the last commit a checkpoint includes and where the log
// continues after it.
//
// A checkpoint of state st at cut x, with st's checkpoint before x and x at
// most st's last commit, keeps these invariants, which recovery relies on:
//
//  1. Its tree under the records after x, replayed in order, is the state as
//     of the last record replayed. Each key whose overlay entry in st lies
//     at or before x takes that entry. Every other key keeps its old tree
//     entry, and the record that last changed it, after x and at most st's
//     last commit, is replayed over it.
//  2. Every record published before saveTree's barrier, with its values, is
//     ordered before the superblock, and that includes every record through
//     st's last commit. So a drive that keeps the superblock keeps each
//     record that shadows an old entry. A value an old entry names may be
//     released, but only once a durable record replaced it, which replay
//     applies over the tree.
//  3. The superblock names the log after record x and the key count as of
//     x, so replay starts at record x+1 and counts from there.
//  4. The pages the tree replaced, the old space record and the log chunks
//     before the one holding record x+1 are tagged x and released only once
//     no snapshot reads an earlier tree and x's superblock is durable. Every
//     later chunk stays.
//  5. The overlay published over the tree holds every entry after x: those
//     the tree left out and those committed since. Only an entry at or
//     before x, which the tree holds, leaves it, and its filter holds every
//     key it holds.
type logCut struct {
	// seq is the last commit the checkpoint includes.
	seq uint64
	// pos is the offset of the record after seq.
	pos uint64
	// end is the end of the chunk holding pos.
	end uint64
	// crc is the checksum of record seq.
	crc uint32
	// count is the number of keys as of seq.
	count int64
	// logBytes is the size of the records after the checkpoint through seq
	// that the state counted.
	logBytes int64
}

// wholeCut returns the cut at st's last commit.
func (st *state) wholeCut() logCut {
	return logCut{seq: st.seq, pos: st.pos, end: st.end, crc: st.crc, count: st.count, logBytes: st.logBytes}
}

// chooseCut returns the cut of a background checkpoint of st: its last
// commit when the overlay fits CheckpointSliceBytes or slicing is off, else
// the commit through which the overlay entries hold about that many bytes or
// half the overlay, whichever is more.
// It reads the log from st's checkpoint through the cut for where the log
// continues and the key count there, so it costs one scan of the overlay and
// one read of the slice's records, and nothing per commit. The snapshot of st
// keeps the log chunks it reads. The caller need not hold the writer lock.
func (w *writer) chooseCut(st *state) (logCut, error) {
	// Take the whole overlay when it fits a slice, and at least half of it
	// otherwise.
	budget := w.db.opts.CheckpointSliceBytes
	if budget < 0 || st.overlayBytes <= budget {
		return st.wholeCut(), nil
	}
	x := sliceSeq(st, max(budget, st.overlayBytes/2))
	if x >= st.seq {
		return st.wholeCut(), nil
	}

	// Read the records after the checkpoint through x.
	sb := st.sb
	c := logCut{seq: x, count: int64(sb.count)} // #nosec G115 -- a file holds under 1<<63 keys.
	lr := newLogReader(w.db.s, sb.logPos, sb.logEnd, sb.seq, sb.logCrc)
	for lr.seq <= x {
		r, ok := lr.next()
		if !ok {
			return logCut{}, errors.Wrapf(ErrCorrupt, "log ends before record %d", x)
		}
		c.logBytes += int64(r.size)
		if r.kind == kindCommit {
			c.count += r.delta
		}
	}
	c.pos, c.end, c.crc = lr.pos, lr.end, lr.crc
	return c, nil
}

// sliceSeq returns the commit through which st's overlay entries hold at
// least budget bytes, or st's last commit when they hold fewer. It sums the
// entries' sizes into sliceBuckets ranges of commits after the checkpoint
// and ends at the first range that reaches budget, so it needs no sort. The
// result follows st's checkpoint.
func sliceSeq(st *state, budget int64) uint64 {
	// Sum the entries by commit range.
	span := st.seq - st.ckpt
	width := (span + sliceBuckets - 1) / sliceBuckets
	sums := make([]int64, (span+width-1)/width)
	st.overlay.Scan(func(it overlayItem) bool {
		seq := min(max(it.e.seq, st.ckpt+1), st.seq)
		sums[(seq-st.ckpt-1)/width] += it.e.size()
		return true
	})

	// End at the first range that reaches budget.
	var total int64
	for i, n := range sums {
		total += n
		if total >= budget {
			return min(st.ckpt+uint64(i+1)*width, st.seq) // #nosec G115 -- i is not negative.
		}
	}
	return st.seq
}

// carry returns the overlay entries of pre, a published state at or after
// t's, that follow t's cut, for the state moved onto t's tree. A whole
// checkpoint collects them by a scan, since few commits follow it. A slice
// copies the overlay and drops each key the tree saved whose entry is still
// the one saved, at or before the cut, so it costs a delete per saved entry
// rather than an insert per kept one, and indexes the kept keys in a new
// filter. It reads only published states, so it needs no writer lock.
func (w *writer) carry(t *tree, pre *state) *carried {
	// Scan the entries after a whole checkpoint. The filter is sized for the
	// current tree, which the new one replaces at about the same size.
	db, x := w.db, t.cut.seq
	limit := db.opts.checkpointLimit(pre.sb)
	if !t.slice() {
		c := newCarried(x, limit)
		pre.overlay.Scan(c.add)
		return c
	}

	// Copy the overlay as derive does, and drop the saved entries. A key a
	// commit changed after the cut keeps its later entry.
	l := db.bcast.Lock()
	o := pre.overlay.Copy()
	l.Unlock()
	bytes := pre.overlayBytes
	for _, ch := range t.saved {
		if it, ok := o.Get(overlayProbe(ch.key)); ok && it.e.seq <= x {
			o.Delete(it)
			bytes -= it.e.size()
		}
	}

	// Index the kept keys.
	f := newFilter(max(limit, bytes))
	o.Scan(func(it overlayItem) bool {
		f.add(it.e.key)
		return true
	})
	return &carried{seq: x, overlay: o, filter: f, bytes: bytes}
}

// nextSlice starts the next background checkpoint once a saved slice ends,
// when the overlay or log it left still calls for one and slicing is on, so a
// large overlay drains slice by slice without waiting for a commit to start
// each. Only a saved slice calls it, so a dropped tree, a failure or a whole
// checkpoint ends the chain and no build repeats without progress.
// It takes the writer lock behind queued writers, so a caller waiting for no
// build to run, such as Compact, takes its turn between slices. It stops at a
// failed checkpoint, which the next commit reports, and at Close.
func (w *writer) nextSlice() {
	// Skip when nothing calls for a slice.
	opts := w.db.opts
	if opts.CheckpointSliceBytes < 0 || w.closing.Load() || !opts.checkpointDue(w.db.cur.Load()) {
		return
	}

	// Start it under the writer lock, checking again there.
	if err := w.lock(context.Background()); err != nil {
		return
	}
	if w.ckptErr == nil && !w.closing.Load() && opts.checkpointDue(w.db.cur.Load()) {
		w.startCheckpoint()
	}
	w.unlock(nil)
}
