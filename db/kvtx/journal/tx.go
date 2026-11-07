package kvtx_journal

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// MaxRecordBytes bounds the writes one record carries. A larger commit sends
// no record and waits for the local flush: the sink then has a gap at its
// seq, where replay stops, and no later commit relies on the sink until the
// local flush passes it.
const MaxRecordBytes = 8 << 20

// ErrHeadKey refuses a write of HeadKey by anything but the Store.
var ErrHeadKey = errors.New("kvtx journal: the head key is reserved")

// tx is a write transaction opened while the marker is live. It records its
// writes in order and commits as one record.
//
// It exposes only kvtx.Tx and kvtx.OrderedCommitTx: a write path an inner
// transaction offers beside Set and Delete, such as a batch apply, would
// bypass the record, so callers see none and use Set and Delete.
type tx struct {
	kvtx.Tx
	s *Store
	// writes are the transaction's changes, in order.
	writes []Write
	// size is the bytes of writes.
	size int
}

// Set sets key and records it.
func (t *tx) Set(ctx context.Context, key, value []byte) error {
	// Only the Store writes the head.
	if bytes.Equal(key, HeadKey) {
		return ErrHeadKey
	}

	// Write, then record.
	if err := t.Tx.Set(ctx, key, value); err != nil {
		return err
	}
	t.writes = append(t.writes, Write{Key: bytes.Clone(key), Value: bytes.Clone(value)})
	t.size += len(key) + len(value)
	return nil
}

// Delete deletes key and records it.
func (t *tx) Delete(ctx context.Context, key []byte) error {
	if err := t.Tx.Delete(ctx, key); err != nil {
		return err
	}
	t.writes = append(t.writes, Write{Key: bytes.Clone(key), Delete: true})
	t.size += len(key)
	return nil
}

// Commit commits the transaction and returns once it is durable, in the sink
// or locally. Under kvtx.WithOrderedCommitContext it returns once committed.
func (t *tx) Commit(ctx context.Context) error {
	seq, err := t.s.commit(ctx, t)
	if err != nil || kvtx.IsOrderedCommitContext(ctx) {
		return err
	}
	return t.s.waitDurable(ctx, seq)
}

// CommitOrdered commits the transaction without waiting for durability. Its
// record is still sent: a crash may lose the commit, and replay restores it
// when the sink holds the record.
func (t *tx) CommitOrdered(ctx context.Context) error {
	_, err := t.s.commit(ctx, t)
	return err
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*tx)(nil)
