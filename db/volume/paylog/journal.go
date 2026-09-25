package paylog

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// AppendJournal durably journals reference graph changes in one index commit,
// which also publishes the pending blocks.
func (s *Store) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	value := appendEdges(nil, adds)
	value = appendEdges(value, removes)
	return s.update(func(b *bbolt.Bucket) error {
		seq := s.journal + 1
		key := binary.BigEndian.AppendUint64([]byte(journalPrefix), seq)
		if err := b.Put(key, value); err != nil {
			return err
		}
		s.journal = seq
		return nil
	})
}

// ReplayJournal passes every journal entry in order to apply and removes the
// entries in one index commit, skipping the commit when the journal is empty.
// Each removal moves the cursor, so the loop seeks the first remaining entry.
func (s *Store) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	prefix := []byte(journalPrefix)
	var empty bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		k, _ := tx.Bucket(bucket).Cursor().Seek(prefix)
		empty = !bytes.HasPrefix(k, prefix)
		return nil
	})
	if err != nil || empty {
		return err
	}
	return s.update(func(b *bbolt.Bucket) error {
		c := b.Cursor()
		for k, v := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, v = c.Seek(prefix) {
			adds, rest, err := readEdges(v)
			if err != nil {
				return err
			}
			removes, _, err := readEdges(rest)
			if err != nil {
				return err
			}
			if err := apply(adds, removes); err != nil {
				return err
			}
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// appendEdges appends a count and each edge's subject and object, every
// value length prefixed with an unsigned varint.
func appendEdges(b []byte, edges []block_gc.RefEdge) []byte {
	b = binary.AppendUvarint(b, uint64(len(edges)))
	for _, e := range edges {
		b = binary.AppendUvarint(b, uint64(len(e.Subject)))
		b = append(b, e.Subject...)
		b = binary.AppendUvarint(b, uint64(len(e.Object)))
		b = append(b, e.Object...)
	}
	return b
}

// readEdges decodes edges written by appendEdges and returns the rest of b.
func readEdges(b []byte) ([]block_gc.RefEdge, []byte, error) {
	n, size := binary.Uvarint(b)
	if size <= 0 || n > uint64(len(b)) {
		return nil, nil, errors.New("invalid journal entry")
	}
	b = b[size:]
	edges := make([]block_gc.RefEdge, n)
	for i := range edges {
		var fields [2]string
		for j := range fields {
			l, size := binary.Uvarint(b)
			if size <= 0 || l > uint64(len(b)-size) { //nolint:gosec
				return nil, nil, errors.New("invalid journal entry")
			}
			end := size + int(l) //nolint:gosec
			fields[j] = string(b[size:end])
			b = b[end:]
		}
		edges[i] = block_gc.RefEdge{Subject: fields[0], Object: fields[1]}
	}
	return edges, b, nil
}
