package s4db

import (
	"context"
)

// releaseStep frees what may be reused: pending chunks that no slot pins and
// recovery can no longer read, and reserved chunks whose owner is gone. It
// appends one free record for them and truncates the file when its trailing
// chunks are free, all under the writer lock; replayers apply the records
// without repeating the slot reads and marks that justified them.
func (s *scheduler) releaseStep(ctx context.Context) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	db := s.db
	return db.w.locked(ctx, func() error {
		// Read the slots: the lowest pin and the live owners.
		st := db.cur.Load()
		pins, readable, err := db.liveSlots()
		if err != nil {
			return err
		}
		p := db.localPin()
		owners := map[uint64]bool{db.id: true}
		for _, sp := range pins {
			p = p.lower(sp)
			owners[sp.id] = true
		}

		// Read the marks: the logged mark and the durable superblock.
		_, logged := db.flush.marks()
		db.superMtx.Lock()
		superSeq := db.super.seq
		db.superMtx.Unlock()

		// Collect the chunks both conditions allow.
		var free []uint64
		for c, e := range st.m.table.entries {
			switch e.kind {
			case chunkPending:
				durable := logged
				if e.owner != chunkValue {
					durable = superSeq
				}
				if p.seq >= e.seq && durable >= e.seq {
					free = append(free, uint64(c)) // #nosec G115 -- a table index.
				}
			case chunkReserved:
				if readable && !owners[e.owner] {
					free = append(free, uint64(c)) // #nosec G115 -- a table index.
				}
			}
		}
		if len(free) != 0 {
			if st, err = db.w.appendRecord(ctx, &record{kind: kindFree, chunks: free}); err != nil {
				return err
			}
		}

		// Truncate the file to its last chunk in use, in this lock span.
		end := st.m.table.end()
		for end > firstChunk+2 && st.m.table.entries[end-1].kind == chunkFree {
			end--
		}
		if end == st.m.table.end() {
			return nil
		}
		if _, err := db.w.appendRecord(ctx, &record{kind: kindTruncate, next: end}); err != nil {
			return err
		}
		return db.s.truncate(fileOff(chunkStart(end)))
	})
}

// liveSlots returns the pins of the other live slots and whether every one
// of them read whole; a slot being rewritten pins everything and blocks
// reclaiming until it reads again.
func (db *DB) liveSlots() ([]pin, bool, error) {
	// Read the slots.
	b := make([]byte, pageSize)
	if _, err := db.s.ReadAt(b, slotPage*pageSize); err != nil {
		return nil, false, err
	}

	// Keep each slot whose process holds its lock.
	var pins []pin
	readable := true
	var empty [slotSize]byte
	for i := range slots {
		rec := b[i*slotSize : (i+1)*slotSize]
		if i == db.slot || [slotSize]byte(rec) == empty {
			continue
		}
		live, err := db.s.held(lockSlot + int64(i))
		if err != nil {
			return nil, false, err
		}
		if !live {
			continue
		}
		sp, ok := decodePin(rec)
		if !ok {
			readable = false
			sp = pin{}
		}
		pins = append(pins, sp)
	}
	return pins, readable, nil
}
