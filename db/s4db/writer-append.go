package s4db

import (
	"context"
	"time"
)

// appendRecord appends r after the published state and publishes the state
// derived from it, returning that state. The caller holds the writer lock;
// r's kind and body are set, and its identities, mark and sequence are
// filled here. A link precedes it when the record does not fit the chunk.
func (w *writer) appendRecord(ctx context.Context, r *record) (*state, error) {
	// Fill the record's identities and mark on a copy of the state. A commit
	// here is a mark, which keeps the open chunks.
	db := w.db
	base := db.cur.Load()
	next := base.derive()
	r.writer, r.prevWriter = db.id, base.writer
	r.durable, _ = db.flush.marks()
	if r.kind == kindCommit {
		r.hot, r.cold = base.m.hot, base.m.cold
	}

	// Place the record after the published state.
	seq := base.seq + 1
	bound := uint64(recordHeader + r.bound()) // #nosec G115 -- sizes are not negative.
	pos, prev := base.pos, base.crc
	var link []byte
	linkPos := pos

	// Continue the log in new chunks when the record may not fit.
	if pos+bound+linkSize > base.end {
		span := max(chunksFor(bound+linkSize), 1)
		l := &record{seq: seq, kind: kindLink, writer: db.id, prevWriter: base.writer, durable: r.durable, span: span}
		l.next = next.m.table.alloc(span, chunkLog, 0)
		link = l.encode(prev)
		next.apply(l, db.budget)
		seq, prev = seq+1, l.crc
		pos = chunkStart(l.next)
		r.prevWriter = db.id
	}
	r.seq = seq
	db.ownSeq.Store(seq)

	// Write the link and the record, then publish.
	rec := r.encode(prev)
	if link != nil {
		if _, err := db.s.WriteAt(link, fileOff(linkPos)); err != nil {
			return nil, err
		}
	}
	if _, err := db.s.WriteAt(rec, fileOff(pos)); err != nil {
		return nil, err
	}
	db.written.Add(int64(len(link) + len(rec)))
	next.apply(r, db.budget)
	if err := db.publish(ctx, next); err != nil {
		return nil, err
	}
	return next, nil
}

// locked runs fn under the writer lock as a background step, recording its
// hold as a commit's wait when a commit waited for the lock meanwhile.
func (w *writer) locked(ctx context.Context, fn func() error) error {
	// Hold the lock around fn and time the hold.
	if err := w.lock(ctx); err != nil {
		return err
	}
	start := time.Now()
	err := fn()
	if w.waiting.Load() != 0 {
		w.db.recordWait(time.Since(start))
	}
	w.unlock()
	return err
}

// reserve allocates n consecutive free chunks for a job of this handle and
// appends the reserve record naming them, returning them.
func (w *writer) reserve(ctx context.Context, n uint64) ([]uint64, error) {
	chunks := make([]uint64, n)
	err := w.locked(ctx, func() error {
		first := w.db.cur.Load().m.table.clone().alloc(n, chunkReserved, w.db.id)
		for i := range chunks {
			chunks[i] = first + uint64(i) // #nosec G115 -- a chunk count.
		}
		_, err := w.appendRecord(ctx, &record{kind: kindReserve, chunks: chunks})
		return err
	})
	return chunks, err
}

// free appends the free record for chunks, when there are any.
func (w *writer) free(ctx context.Context, chunks []uint64) error {
	if len(chunks) == 0 {
		return nil
	}
	return w.locked(ctx, func() error {
		_, err := w.appendRecord(ctx, &record{kind: kindFree, chunks: chunks})
		return err
	})
}
