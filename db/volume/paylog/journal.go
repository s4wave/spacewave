package paylog

import (
	"bytes"
	"context"
	"encoding/binary"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/journal"
)

// AppendJournal durably journals reference graph changes in one index commit,
// which also publishes the pending blocks.
func (s *Store) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return s.appendJournal(ctx, adds, removes, false)
}

// AppendJournalOrdered journals reference graph changes in one ordered index
// commit, which the next Sync or durable commit makes durable.
func (s *Store) AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return s.appendJournal(ctx, adds, removes, true)
}

// appendJournal journals reference graph changes in one index commit, ordered
// if ordered is set.
func (s *Store) appendJournal(ctx context.Context, adds, removes []block_gc.RefEdge, ordered bool) error {
	// Skip empty batches and encode the entry once.
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	value := journal.Marshal(adds, removes)

	// Append the sequence-numbered entry in one index commit.
	return s.update(ctx, ordered, func(tx kvtx.Tx) error {
		// Write the next sequence-numbered journal entry.
		seq := s.journal + 1
		key := binary.BigEndian.AppendUint64([]byte(journalPrefix), seq)
		if err := tx.Set(ctx, key, value); err != nil {
			return err
		}
		s.journal = seq
		return nil
	})
}

// ReplayJournal passes every journal entry in order to apply and removes the
// entries in one index commit, skipping the commit when the journal is empty.
func (s *Store) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	// Read the entries.
	var keys, values [][]byte
	err := s.view(ctx, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte(journalPrefix), true, false)
		defer it.Close()
		for it.Next() {
			value, err := it.ValueCopy(nil)
			if err != nil {
				return err
			}
			keys = append(keys, bytes.Clone(it.Key()))
			values = append(values, value)
		}
		return it.Err()
	})
	if err != nil || len(keys) == 0 {
		return err
	}

	// Apply and remove them.
	return s.update(ctx, false, func(tx kvtx.Tx) error {
		for i, key := range keys {
			adds, removes, err := journal.Unmarshal(values[i])
			if err != nil {
				return err
			}
			if err := apply(adds, removes); err != nil {
				return err
			}
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})
}
