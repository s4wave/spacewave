package s4db

// logZeroes bounds the allocation buffer independently of the log extent.
var logZeroes = make([]byte, 64*1024)

// backLog materializes a log extent before any published link can reach it.
// A logical reservation or truncate is insufficient on a full filesystem:
// every byte used by the next link or emergency free record must already exist.
func backLog(s storage, first, span uint64) error {
	// Touch only chunks that the authoritative table has allocated to the log.
	for off, end := chunkStart(first), chunkStart(first+span); off < end; {
		n := min(uint64(len(logZeroes)), end-off)
		if _, err := s.WriteAt(logZeroes[:n], fileOff(off)); err != nil {
			return err
		}
		off += n
	}
	return nil
}

// backLog accounts for the log backing IO charged to this writer. Creation
// and bulk build use the storage primitive before runtime counters exist.
func (w *writer) backLog(first, span uint64) error {
	w.db.written.Add(fileOff(span * chunkSize))
	return backLog(w.db.s, first, span)
}

// reclaimRecord reports records allowed to spend the standby without replacing
// it first. Free batches are bounded; a quiet mark or deletion can establish the
// durability and unreachability needed to reclaim more chunks on a full disk.
func reclaimRecord(r *record) bool {
	if r.kind == kindFree || r.kind == kindSealed || r.kind == kindTruncate {
		return true
	}
	if r.kind != kindCommit {
		return false
	}
	for _, o := range r.ops {
		if o.val.kind != valueDeleted {
			return false
		}
	}
	return true
}

// placeLog places r and an optional link in next, derived from base. Ordinary
// records retain a fully backed standby. A bounded reclaim record may consume
// that standby without allocating; the first subsequent ordinary record replaces
// it from the newly reusable space before publishing any work.
func (w *writer) placeLog(base, next *state, r *record) (uint64, []byte, error) {
	// Select the next extent, consuming the standby only for a single chunk.
	pos, seq, prev := base.pos, base.seq+1, base.crc
	spare := base.spare
	bound := uint64(recordHeader + r.bound()) // #nosec G115 -- sizes are nonnegative.
	var link []byte
	if pos+bound+linkSize > base.end {
		span := max(chunksFor(bound+linkSize), 1)
		var first uint64
		if span == 1 && spare != 0 {
			first, spare = spare, 0
		} else {
			first = next.m.table.alloc(span, chunkLog, 0)
			if err := w.backLog(first, span); err != nil {
				return 0, nil, err
			}
		}

		// Ordinary work must replace a consumed standby before installing its link.
		if spare == 0 && !reclaimRecord(r) {
			spare = next.m.table.alloc(1, chunkLog, 0)
			if err := w.backLog(spare, 1); err != nil {
				return 0, nil, err
			}
		}
		l := &record{
			seq: seq, kind: kindLink, writer: w.db.id, prevWriter: base.writer,
			durable: r.durable, next: first, span: span, spare: spare,
		}
		link = l.encode(prev)
		next.apply(l, w.db.budget)
		seq, pos = seq+1, chunkStart(first)
		r.prevWriter = w.db.id
	}

	// A reclaim record may have consumed the standby on an earlier call.
	if spare == 0 && !reclaimRecord(r) {
		spare = next.m.table.alloc(1, chunkLog, 0)
		if err := w.backLog(spare, 1); err != nil {
			return 0, nil, err
		}
	}
	r.seq, r.spare = seq, spare
	w.db.ownSeq.Store(seq)
	return pos, link, nil
}
