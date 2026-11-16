package block_gc

import "context"

// tail indexes the blocks the journal records past the cursor name, in an up
// pair or an again list, so the free keeps a block a record it has not netted
// yet names. A writer that names a block it does not hold, such as one put
// again over a stored block or a new parent naming a stored child, can land
// after the block reached zero and took its ripe sequence; the record's
// sequence is past that ripe sequence, so neither ripeness nor the cursor
// waits for it, and only this index keeps the block until the record nets.
//
// Invariant: when the free reads has, named counts exactly the up and again
// entries of the records from the cursor to NextSeq: sync reached NextSeq in
// the free's own transaction, and no Land can commit between that read and
// the free's commit: the collector's store opens a write transaction,
// NewTransaction(ctx, true), which every kvtx backend runs under the same
// single writer lock each Land takes. A sync reads a bounded slice of records
// and keeps its place across transactions; a slice whose sync stops short of
// NextSeq frees nothing. A netting that deletes records trims their entries
// only once its transaction commits; an index that disagrees with the state's
// cursor or adoption run is rebuilt from the cursor before it is read. An
// entry it keeps too long only delays a free.
//
// It holds only the key multiset and streams the records it reads. A
// collector starts with none, so the first frees after a start read the whole
// tail once, a slice at a time; later frees read only the records appended
// since. Past limit entries it is saturated: it drops the keys, keeps only
// the entry count, and names every block, so the free keeps everything until
// netting trims the tail below limit and a rebuild from the cursor fits.
type tail struct {
	// built reports that named holds a reading of the journal under run, from
	// from up to next, which may stop short of NextSeq.
	built bool
	// run is the adoption run the reading was taken under.
	run string
	// from is the cursor the reading starts at.
	from uint64
	// next is the sequence past the last record the reading took.
	next uint64
	// named counts the up and again entries naming each block key in the
	// records from..next; nil while saturated.
	named map[string]int
	// entries counts the up and again entries of the records from..next.
	entries int
	// limit is the entries past which the index saturates.
	limit int
	// saturated reports that entries passed limit, so has names every key.
	saturated bool
	// trim is the trim the transaction in progress would make, applied by
	// commit once that transaction commits.
	trim tailTrim
}

// tailTrim is one netting transaction's deletion of the records from the
// cursor from up to to, with the entries of those the index had read.
type tailTrim struct {
	// set reports that the transaction moved the cursor.
	set bool
	// from is the cursor before the transaction.
	from uint64
	// to is the cursor after it.
	to uint64
	// keys are the up and again entries of the deleted records the index
	// had read, one per entry.
	keys []string
}

// tailNames returns the entries of r the tail index counts: its up pairs and
// its again list. sync and the netting's trim both read them here, so a trim
// takes out exactly what the sync put in.
func tailNames(r *JournalRecord) [2][][]byte {
	return [2][][]byte{r.GetUp(), r.GetAgain()}
}

// has reports whether a record past the cursor names key, true for every key
// while saturated. It is exact only after a sync that reached NextSeq in the
// same transaction.
func (t *tail) has(key []byte) bool {
	return t.saturated || t.named[string(key)] != 0
}

// sync extends the index toward state's NextSeq in rec, the free's
// transaction: from the last record it read, or from the cursor when it holds
// no reading of state's journal or a saturated one netting trimmed below the
// limit. It reads whole records, stopping before the one that would pass
// budget entries, counting at least one per record, unless it read none yet.
// It reports whether it reached NextSeq.
func (t *tail) sync(ctx context.Context, rec *Records, state *LivenessState, budget int) (bool, error) {
	// Start over from the cursor when the reading is not of this journal or
	// a rebuild now fits.
	cursor := state.GetCursor()
	if !t.built || t.run != state.GetAdoptionRun() || t.from != cursor || (t.saturated && t.entries <= t.limit) {
		t.built, t.run, t.from, t.next = true, state.GetAdoptionRun(), cursor, cursor
		t.entries, t.saturated = 0, false
		if t.named == nil {
			t.named = make(map[string]int)
		}
		clear(t.named)
	}

	// Stream the records from next into the counts, up to the budget.
	read, reached := 0, true
	err := rec.ScanJournal(ctx, t.next, func(seq uint64, r *JournalRecord) (bool, error) {
		// Stop before the record that would pass the budget.
		cost := max(len(r.GetUp())+len(r.GetDown())+len(r.GetAgain()), 1)
		if read != 0 && read+cost > budget {
			reached = false
			return false, nil
		}
		read += cost

		// Count its names, dropping the keys once they pass the limit.
		names := tailNames(r)
		t.entries += len(names[0]) + len(names[1])
		if t.entries > t.limit && !t.saturated {
			t.saturated, t.named = true, nil
		}
		if !t.saturated {
			for _, keys := range names {
				for _, key := range keys {
					t.named[string(key)]++
				}
			}
		}
		t.next = seq + 1
		return true, nil
	})
	if err != nil {
		t.built = false
		return false, err
	}
	if reached {
		t.next = max(t.next, state.GetNextSeq())
	}
	return reached, nil
}

// read reports whether the index holds the record seq, so a netting that
// deletes it records its entries in the trim.
func (t *tail) read(seq uint64) bool {
	return t.built && seq >= t.from && seq < t.next
}

// begin starts a transaction with no trim.
func (t *tail) begin() {
	t.trim = tailTrim{}
}

// commit applies the trim of the transaction that just committed. A trim
// that does not start at the index's cursor drops the reading, which the
// next free rebuilds.
func (t *tail) commit() {
	// Apply nothing for a transaction that moved no cursor.
	tr := t.trim
	t.trim = tailTrim{}
	if !tr.set || !t.built {
		return
	}
	if tr.from != t.from {
		t.built = false
		return
	}

	// Take the deleted records' entries out of the counts.
	t.entries -= len(tr.keys)
	if !t.saturated {
		for _, key := range tr.keys {
			if t.named[key]--; t.named[key] <= 0 {
				delete(t.named, key)
			}
		}
	}
	t.from, t.next = tr.to, max(t.next, tr.to)
}
