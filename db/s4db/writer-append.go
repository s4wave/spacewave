package s4db

import "context"

// appendRecord appends r after the published state and publishes the state
// derived from it, returning that state. The caller holds the writer lock.
// r's kind and body are set, and placement fills its header. A link precedes
// it when the record does not fit the chunk.
func (w *writer) appendRecord(ctx context.Context, r *record) (*state, error) {
	// Reserve the producer's chunks on a copy of the state before allocating
	// log or standby space. The reservation record has not yet been applied
	// to the public state.
	db := w.db
	base := db.cur.Load()
	next := base.derive()
	if r.kind == kindReserve {
		next.reserve(r.chunks, db.id)
	}

	// Place, write and publish the record.
	if err := w.append(ctx, base, next, r, nil); err != nil {
		return nil, err
	}
	return next, nil
}

// mark appends a mark record: a commit with no changes, which carries the
// writer's durable mark and keeps the open value chunks. The caller holds the
// writer lock.
func (w *writer) mark(ctx context.Context) error {
	_, err := w.appendRecord(ctx, newCommit(w.db.cur.Load().m))
	return err
}

// locked runs one scheduler slice through the same coordinator as transactions.
// Its hold contributes to the longest foreground wait when demand exists.
func (w *writer) locked(ctx context.Context, fn func() error) error {
	// Take one background grant. Per-caller aging cannot multiply this
	// turn.
	if err := w.take(ctx, true); err != nil {
		return err
	}
	defer w.unlock()

	// Run only the admitted slice while retaining native writer exclusion.
	return fn()
}

// reserve allocates n consecutive free chunks for a job of this handle and
// appends the reserve record listing them, returning them.
func (w *writer) reserve(ctx context.Context, n uint64) ([]uint64, error) {
	chunks := make([]uint64, n)
	err := w.locked(ctx, func() error {
		first := w.db.cur.Load().freeRun(n)
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
			unused := w.db.cur.Load().reservedBy(batch, w.db.id)
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
