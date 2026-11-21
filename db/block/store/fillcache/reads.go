package block_store_fillcache

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// readsTable records, for each row of the cache table, the read sequence of
// its last fill or hit, so eviction deletes the rows read longest ago first.
//
// The sequence grows by one for each batch the writer stamps and resumes from
// the table's largest value when the file opens, so the order holds across
// restarts: the blocks a start reads are newer than every row it did not read.
// A row with no entry was never stamped, as rows written before the table
// existed or whose stamp failed, and eviction takes it before any stamped row.
// An entry whose row is gone, as after a removal, is deleted by the eviction
// that reaches it.
const readsTable = "fill_reads"

// pendingReadsMax bounds the hits awaiting their stamp. A hit past it goes
// unstamped, which only lets its row look older than it is.
const pendingReadsMax = 1 << 16

// createReads creates the reads table and its sequence index when missing.
func createReads(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS "+readsTable+" (key BLOB PRIMARY KEY NOT NULL, seq INTEGER NOT NULL) WITHOUT ROWID;"+
			"CREATE INDEX IF NOT EXISTS "+readsTable+"_seq ON "+readsTable+" (seq)",
	)
	return errors.Wrap(err, "create the fill cache reads table")
}

// lastReadSeq returns the largest read sequence the file holds, or zero.
func lastReadSeq(ctx context.Context, db *sql.DB) (int64, error) {
	var seq int64
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq), 0) FROM "+readsTable).Scan(&seq) //nolint:gosec // the table name is the cache's constant.
	return seq, errors.Wrap(err, "read the fill cache read sequence")
}

// touch queues the stamp of a hit's row. It runs on the read path, so it only
// adds the key to a bounded set the writer takes; the first key wakes the
// writer.
func (c *Cache) touch(ref *block.BlockRef) {
	key, err := c.evict.blockKey(ref)
	if err != nil {
		return
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if len(c.reads) >= pendingReadsMax {
			return
		}
		c.reads[string(key)] = struct{}{}
		if len(c.reads) == 1 {
			broadcast()
		}
	})
}

// stamp records the read sequence seq for the rows of the written fills and of
// the hits in one transaction.
func (c *Cache) stamp(ctx context.Context, seq int64, fills []*block.PutBatchEntry, hits map[string]struct{}) error {
	// Collect the keys of the written blocks.
	keys := make([][]byte, 0, len(fills)+len(hits))
	for _, entry := range fills {
		if entry.Tombstone {
			continue
		}
		key, err := c.evict.blockKey(entry.Ref)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	for key := range hits {
		keys = append(keys, []byte(key))
	}
	if len(keys) == 0 {
		return nil
	}

	// Set each key's sequence.
	tx, err := c.evict.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO "+readsTable+" (key, seq) VALUES (?, ?) "+ //nolint:gosec // the table name is the cache's constant.
			"ON CONFLICT (key) DO UPDATE SET seq = excluded.seq",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, key := range keys {
		if _, err := stmt.ExecContext(ctx, key, seq); err != nil {
			return err
		}
	}
	return tx.Commit()
}
