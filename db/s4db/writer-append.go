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

	// Reserve the producer's chunks before allocating log or standby space.
	// The reservation record has not yet been applied to the public state.
	if r.kind == kindReserve {
		for _, c := range r.chunks {
			*next.m.table.at(c) = chunkEntry{kind: chunkReserved, owner: db.id}
		}
	}
	r.writer, r.prevWriter = db.id, base.writer
	r.durable, _ = db.flush.marks()
	if r.kind == kindCommit {
		r.hot, r.cold = base.m.hot, base.m.cold
	}

	// Place the record while keeping backed space for full-disk reclamation.
	pos, link, err := w.placeLog(base, next, r)
	if err != nil {
		return nil, err
	}
	linkPos, prev := base.pos, base.crc
	if link != nil {
		prev = next.crc
	}

	// A truncate target was chosen before placing this record. A rollover may
	// have claimed a chunk at or above that target; keep the log and standby.
	if r.kind == kindTruncate {
		r.next = max(r.next, chunksFor(next.end), r.spare+1)
	}

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
	for {
		// Check announcements under the writer lock. Once this check passes,
		// a newly arriving request can wait behind only this one step.
		if err := w.take(ctx); err != nil {
			return err
		}
		slot, err := w.waitingSlot()
		if err != nil {
			w.unlock()
			return err
		}
		if slot < 0 {
			start := time.Now()
			err = fn()
			if w.waiting.Load() != 0 {
				w.db.recordWait(time.Since(start))
			}
			w.unlock()
			return err
		}

		// Wait without either writer lock until the announced request takes
		// its turn, is cancelled locally, or its remote process exits.
		w.unlock()
		if err := w.waitRequest(ctx, slot); err != nil {
			return err
		}
	}
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

// freeBatch bounds each reclaim record to well below the standby chunk.
const freeBatch = 256

// free returns still-unused reservations in bounded writer steps. Retrying a
// partial cleanup cannot free chunks that have since become live run output.
func (w *writer) free(ctx context.Context, chunks []uint64) error {
	for len(chunks) != 0 {
		batch := chunks[:min(len(chunks), freeBatch)]
		if err := w.locked(ctx, func() error {
			// Completed run/manifest output is never disposable reservation cleanup.
			table := w.db.cur.Load().m.table
			var unused []uint64
			for _, c := range batch {
				if c < table.end() && table.at(c).kind == chunkReserved && table.at(c).owner == w.db.id {
					unused = append(unused, c)
				}
			}
			if len(unused) == 0 {
				return nil
			}
			_, err := w.appendRecord(ctx, &record{kind: kindFree, chunks: unused})
			return err
		}); err != nil {
			return err
		}
		chunks = chunks[len(batch):]
	}
	return nil
}
