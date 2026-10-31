package s4db

import (
	"context"
	"math"
)

// cleanSlackDiv sets the slack bound: free chunks plus dead bytes stay under
// the live bytes divided by it. Tests lower the bound through cleanSlack.
var cleanSlackDiv int64 = 8

// cleanUrgentDiv sets the urgent bound, past which the cleaner skips the
// quiet gate: slack over the live bytes divided by it.
const cleanUrgentDiv = 2

// cleanBatch bounds the value bytes one cleaner commit moves.
const cleanBatch = 256 << 10

// cleaner evacuates value chunks whose live fraction fell, moving their live
// entries into the cold chunk through move commits, and seals emptied chunks,
// as the design's Clean path describes. It reads a victim on a snapshot off
// the lock and takes the lock only for its records.
type cleaner struct {
	// db is the database.
	db *DB
	// stuck holds the chunks a scan found damaged, with the seq each held
	// then, so the cleaner passes over them until a commit writes them again.
	stuck map[uint64]uint64
}

// slack returns the live bytes and the slack: free chunks plus dead bytes in
// value chunks.
func (m *manifest) slack() (live, slack int64) {
	for c, e := range m.table.entries {
		if c == 0 {
			continue
		}
		switch e.kind {
		case chunkFree:
			slack += chunkSize
		case chunkValue:
			live += int64(e.live)
			slack += int64(e.used) - int64(e.live)
		}
	}
	return live, slack
}

// due reports whether a step is due, and whether it is urgent: the slack
// passed its bound, or a value chunk sits at the top of a file with free
// chunks below it.
func (c *cleaner) due(m *manifest) (urgent, ok bool) {
	// The slack bounds first, then the top of the file.
	live, slack := m.slack()
	if slack > live/cleanUrgentDiv && slack >= chunkSize {
		return true, true
	}
	if slack > live/cleanSlackDiv && slack >= chunkSize {
		return false, true
	}
	_, top := c.topVictim(m)
	return false, top
}

// topVictim returns the value chunk holding the file's last chunk when free
// chunks sit below it and a scan has not found it stuck.
func (c *cleaner) topVictim(m *manifest) (uint64, bool) {
	// The last chunk must be a closed value chunk with a free chunk below.
	t := m.table
	end := t.end()
	if end <= firstChunk || t.entries[end-1].kind != chunkValue || end-1 == m.hot || end-1 == m.cold {
		return 0, false
	}
	if seq, stuck := c.stuck[end-1]; stuck && seq == t.entries[end-1].seq {
		return 0, false
	}
	for ch := uint64(firstChunk); ch < end-1; ch++ {
		if t.entries[ch].kind == chunkFree {
			return end - 1, true
		}
	}
	return 0, false
}

// victim chooses the chunk to clean: the top value chunk when free chunks sit
// below it, else the closed value chunk with the best cost-benefit over its
// dead fraction and age, never the open chunks or a stuck chunk.
func (c *cleaner) victim(m *manifest) (uint64, bool) {
	// The top of the file first.
	if top, ok := c.topVictim(m); ok {
		return top, true
	}

	// Cost-benefit, Sprite LFS's (1 - u) * age / (1 + u), with age as the
	// records since the chunk was last written.
	best, bestScore := uint64(0), -1.0
	now := float64(m.version)
	for ch, e := range m.table.entries {
		if e.kind != chunkValue || uint64(ch) == m.hot || uint64(ch) == m.cold || e.used == 0 { // #nosec G115 -- a table index.
			continue
		}
		if seq, stuck := c.stuck[uint64(ch)]; stuck && seq == e.seq { // #nosec G115 -- a table index.
			continue
		}
		u := float64(e.live) / float64(e.used)
		score := (1 - u) * math.Max(now-float64(e.seq), 1) / (1 + u)
		if e.live == 0 {
			score = math.Inf(1)
		}
		if score > bestScore {
			best, bestScore = uint64(ch), score // #nosec G115 -- a table index.
		}
	}
	return best, bestScore > 0
}

// step cleans one victim: scans it on a snapshot, commits its live entries
// into the cold chunk in batches, and seals it once empty. The caller holds
// lockClean.
func (c *cleaner) step(ctx context.Context) error {
	// Choose the victim on a snapshot.
	db := c.db
	st, stripe := db.acquire()
	victim, ok := c.victim(st.m)
	if !ok {
		db.release(st, stripe)
		return nil
	}
	e := *st.m.table.at(victim)

	// A chunk with no live entry is sealed without a scan.
	if e.live == 0 {
		db.release(st, stripe)
		return c.seal(ctx, victim)
	}

	// Read and parse the victim off the lock, on the idle thread, and find
	// the entries the snapshot still points at.
	var entries []valueEntry
	var readErr error
	db.sched.idle.do(ctx, func() {
		b := make([]byte, chunkSize)
		n, err := db.s.ReadAt(b, fileOff(chunkStart(victim)))
		if err != nil && n == 0 {
			readErr = err
			return
		}
		entries = parseValueEntries(b[:n], chunkStart(victim))
	})
	if readErr != nil {
		db.release(st, stripe)
		return readErr
	}
	live, err := c.liveEntries(st, entries)
	db.release(st, stripe)
	if err != nil {
		return err
	}
	if len(live) == 0 && e.live != 0 {
		// The table says live bytes remain but no entry parses as live: the
		// chunk holds part of a value larger than a chunk, is damaged, or is
		// still being written; pass over it until a commit writes it again.
		c.remember(victim, e.seq)
		return nil
	}

	// Move the live entries in batches under the lock. A move applies only
	// while the key still holds the reference read, which the commit checks
	// on the state the lock made current.
	for len(live) != 0 {
		batch, rest := c.batch(live)
		live = rest
		if err := db.w.locked(ctx, func() error {
			return db.w.commit(ctx, db.cur.Load(), batch, true)
		}); err != nil {
			return err
		}
		if db.WritersWaiting() {
			return nil
		}
	}

	// Seal the victim when its live count reached zero.
	if db.cur.Load().m.table.at(victim).live == 0 {
		return c.seal(ctx, victim)
	}
	return nil
}

// liveEntries returns, as move changes, the entries of a victim whose keys
// still point at them in st.
func (c *cleaner) liveEntries(st *state, entries []valueEntry) ([]tentry, error) {
	var live []tentry
	for _, e := range entries {
		v, found, err := c.db.lookup(st, e.key)
		if err != nil {
			return nil, err
		}
		if found && v.isRef() && v.ref.off == e.off {
			live = append(live, tentry{key: e.key, data: e.data, expect: e.off})
		}
	}
	return live, nil
}

// batch splits off the first changes up to cleanBatch bytes.
func (c *cleaner) batch(live []tentry) (batch, rest []tentry) {
	n, bytes := 0, 0
	for n < len(live) && (n == 0 || bytes+len(live[n].data) <= cleanBatch) {
		bytes += len(live[n].data)
		n++
	}
	return live[:n], live[n:]
}

// seal appends the sealed record for an emptied chunk, then runs the release
// step.
func (c *cleaner) seal(ctx context.Context, victim uint64) error {
	db := c.db
	err := db.w.locked(ctx, func() error {
		if e := db.cur.Load().m.table.at(victim); e.kind != chunkValue || e.live != 0 {
			return nil
		}
		_, err := db.w.appendRecord(ctx, &record{kind: kindSealed, chunks: []uint64{victim}})
		return err
	})
	if err != nil {
		return err
	}
	return db.sched.releaseStep(ctx)
}

// remember marks a chunk stuck at its current seq.
func (c *cleaner) remember(victim, seq uint64) {
	if c.stuck == nil {
		c.stuck = make(map[uint64]uint64)
	}
	c.stuck[victim] = seq
}
