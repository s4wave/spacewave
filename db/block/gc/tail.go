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
// entries of the records from the cursor to NextSeq, read in the free's own
// transaction. The free extends it to NextSeq inside that transaction, and
// no Land can commit between the extension and the free's commit: the
// collector's store opens a write transaction, NewTransaction(ctx, true),
// which every kvtx backend runs under the same single writer lock each Land
// takes. A netting that deletes records trims their entries only once its
// transaction commits; an index that disagrees with the state's cursor or
// adoption run is rebuilt from the cursor before it is read. An entry it
// keeps too long only delays a free.
//
// It holds only the key multiset and streams the records it reads. A
// collector starts with none, so the first free after a start reads the
// whole tail once; later frees read only the records appended since.
type tail struct {
	// built reports that named holds a reading of the journal under run.
	built bool
	// run is the adoption run the reading was taken under.
	run string
	// from is the cursor the reading starts at.
	from uint64
	// next is the sequence past the last record the reading took.
	next uint64
	// named counts the up and again entries naming each block key in the
	// records from..next.
	named map[string]int
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

// has reports whether a record past the cursor names key. It is exact only
// after sync in the same transaction.
func (t *tail) has(key []byte) bool {
	return t.named[string(key)] != 0
}

// sync brings the index up to state's NextSeq in rec, the free's
// transaction: from the last record it read, or from the cursor when it holds
// no reading of state's journal.
func (t *tail) sync(ctx context.Context, rec *Records, state *LivenessState) error {
	// Start over from the cursor when the reading is not of this journal.
	cursor := state.GetCursor()
	if !t.built || t.run != state.GetAdoptionRun() || t.from != cursor {
		t.built, t.run, t.from, t.next = false, state.GetAdoptionRun(), cursor, cursor
		clear(t.named)
	}
	if t.named == nil {
		t.named = make(map[string]int)
	}

	// Stream the records from next to the journal's end into the counts.
	err := rec.ScanJournal(ctx, t.next, func(seq uint64, r *JournalRecord) (bool, error) {
		for _, keys := range [][][]byte{r.GetUp(), r.GetAgain()} {
			for _, key := range keys {
				t.named[string(key)]++
			}
		}
		t.next = seq + 1
		return true, nil
	})
	if err != nil {
		t.built = false
		return err
	}
	t.next, t.built = max(t.next, state.GetNextSeq()), true
	return nil
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
	for _, key := range tr.keys {
		if t.named[key]--; t.named[key] <= 0 {
			delete(t.named, key)
		}
	}
	t.from, t.next = tr.to, max(t.next, tr.to)
}
