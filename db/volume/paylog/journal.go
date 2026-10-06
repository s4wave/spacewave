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

// ReplayJournal passes every journal entry in order to apply, outside any
// index transaction, and then removes them in one durable index commit. A
// crash before the removal passes the same entries again, so apply must be
// idempotent over the sequence.
func (s *Store) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	// Read the entries.
	var keys, values [][]byte
	err := s.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(journalPrefix), func(key, value []byte) error {
			keys, values = append(keys, bytes.Clone(key)), append(values, bytes.Clone(value))
			return nil
		})
	})
	if err != nil || len(keys) == 0 {
		return err
	}

	// Apply them in order.
	for _, value := range values {
		adds, removes, err := journal.Unmarshal(value)
		if err != nil {
			return err
		}
		if err := apply(adds, removes); err != nil {
			return err
		}
	}

	// Remove them.
	return s.update(ctx, false, func(tx kvtx.Tx) error {
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})
}
