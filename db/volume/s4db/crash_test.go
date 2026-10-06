//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/journal"
)

// journalPrefix holds the crash target's journal entries by sequence.
const journalPrefix = "j/"

// TestCrashRecovery crashes the volume's block store and key-value store on
// one database at every device call of the crash workload.
func TestCrashRecovery(t *testing.T) {
	crashtest.Run(t, crashtest.ImmediateBlocks, device.NewMemory, func(ctx context.Context, d *device.Memory) (crashtest.Target, error) {
		db, err := s4db.OpenDevice(ctx, d, "volume.s4wave", s4db.Options{})
		if err != nil {
			return nil, err
		}
		return &crashTarget{
			Store:     kvtx_prefixer.NewPrefixer(db, []byte("k/")),
			KVTxBlock: block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), db, 0, true),
			db:        db,
		}, nil
	})
}

// crashTarget is the volume's block store and a key-value store on one
// database, with a journal beside them.
type crashTarget struct {
	kvtx.Store
	*block_store_kvtx.KVTxBlock

	// db is the database.
	db *s4db.DB
}

// Sync makes every earlier commit durable, as the volume's Sync does.
func (c *crashTarget) Sync(ctx context.Context) (bool, error) {
	return true, c.db.Sync(ctx)
}

// AppendJournal journals reference graph changes in one durable commit.
func (c *crashTarget) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return c.appendJournal(ctx, adds, removes, false)
}

// AppendJournalOrdered journals reference graph changes in one ordered commit.
func (c *crashTarget) AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return c.appendJournal(ctx, adds, removes, true)
}

// appendJournal journals reference graph changes after the last entry, with
// write ordering only if ordered is set.
func (c *crashTarget) appendJournal(ctx context.Context, adds, removes []block_gc.RefEdge, ordered bool) error {
	// Open the write transaction.
	tx, err := c.db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Find the last journal sequence.
	var seq uint64
	it := tx.Iterate(ctx, []byte(journalPrefix), true, true)
	if it.Next() {
		seq = binary.BigEndian.Uint64(it.Key()[len(journalPrefix):])
	}
	err = it.Err()
	it.Close()
	if err != nil {
		return err
	}

	// Store the entry at the next sequence and commit.
	key := binary.BigEndian.AppendUint64([]byte(journalPrefix), seq+1)
	if err := tx.Set(ctx, key, journal.Marshal(adds, removes)); err != nil {
		return err
	}
	if ordered {
		return kvtx.CommitOrdered(ctx, tx)
	}
	return tx.Commit(ctx)
}

// ReplayJournal passes every journal entry in order to apply and removes the
// entries in one durable commit.
func (c *crashTarget) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	// Open the write transaction.
	tx, err := c.db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Apply each entry in sequence order and collect its key.
	var keys [][]byte
	err = tx.ScanPrefix(ctx, []byte(journalPrefix), func(key, value []byte) error {
		adds, removes, err := journal.Unmarshal(value)
		if err != nil {
			return err
		}
		keys = append(keys, bytes.Clone(key))
		return apply(adds, removes)
	})
	if err != nil {
		return err
	}

	// Delete the applied entries and commit.
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Close closes the database.
func (c *crashTarget) Close() error {
	return c.db.Close()
}

// _ is a type assertion
var _ crashtest.Target = (*crashTarget)(nil)
