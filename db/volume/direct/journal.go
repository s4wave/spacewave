package direct

import (
	"bytes"
	"context"
	"encoding/binary"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/journal"
)

// AppendJournal durably journals reference graph changes in one commit, which
// also commits the pending blocks.
func (s *Store) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return s.appendJournal(ctx, adds, removes, false)
}

// AppendJournalOrdered journals reference graph changes in one ordered
// commit, which the next Sync or durable commit makes durable.
func (s *Store) AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return s.appendJournal(ctx, adds, removes, true)
}

// appendJournal journals reference graph changes in one commit, ordered if
// ordered is set.
func (s *Store) appendJournal(ctx context.Context, adds, removes []block_gc.RefEdge, ordered bool) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	value := journal.Marshal(adds, removes)
	return s.update(ctx, ordered, func(tx kvtx.Tx) error {
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
// entries in one durable commit, skipping the commit when the journal is
// empty.
func (s *Store) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	// Read the entries.
	var keys, values [][]byte
	err := s.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(journalPrefix), func(key, value []byte) error {
			keys, values = append(keys, bytes.Clone(key)), append(values, value)
			return nil
		})
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
