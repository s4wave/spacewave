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
// restarts. A row with no entry was never stamped, as rows written before the
// table existed or whose stamp failed, and eviction takes it before any stamped
// row. An entry whose row is gone, as after a removal, is deleted by the
// eviction that reaches it.
//
// Invariant: rows stamped in the latest completed start window, the range
// metaTable holds, are evicted only after every other row. A start's blocks
// move to the decoded-block pool after their first hit, and pool hits never
// reach the cache, so by sequence alone they would soon look the oldest.
const readsTable = "fill_reads"

// metaTable holds the cache's named integers: the protected start range, from
// metaStartLo to metaStartHi inclusive.
const metaTable = "fill_meta"

// metaStartLo and metaStartHi name the bounds of the protected start range.
const (
	metaStartLo = "start_lo"
	metaStartHi = "start_hi"
)

// pendingReadsMax bounds the hits awaiting their stamp. A hit past it goes
// unstamped, which only lets its row look older than it is.
const pendingReadsMax = 1 << 16

// createReads creates the reads table, its sequence index and the meta table
// when missing.
func createReads(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS "+readsTable+" (key BLOB PRIMARY KEY NOT NULL, seq INTEGER NOT NULL) WITHOUT ROWID;"+
			"CREATE INDEX IF NOT EXISTS "+readsTable+"_seq ON "+readsTable+" (seq);"+
			"CREATE TABLE IF NOT EXISTS "+metaTable+" (name TEXT PRIMARY KEY NOT NULL, value INTEGER NOT NULL) WITHOUT ROWID",
	)
	return errors.Wrap(err, "create the fill cache reads table")
}

// startRange returns the protected start range the file holds, and false when
// it holds none.
func startRange(ctx context.Context, db *sql.DB) (lo, hi int64, ok bool, err error) {
	var n int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(MAX(CASE name WHEN ? THEN value END), 0), COALESCE(MAX(CASE name WHEN ? THEN value END), 0) "+
			"FROM "+metaTable+" WHERE name IN (?, ?)", //nolint:gosec // the table name is the cache's constant.
		metaStartLo, metaStartHi, metaStartLo, metaStartHi,
	).Scan(&n, &lo, &hi)
	if err != nil {
		return 0, 0, false, errors.Wrap(err, "read the fill cache start range")
	}
	return lo, hi, n == 2 && lo <= hi, nil
}

// storeStartRange replaces the protected start range in one transaction.
func storeStartRange(ctx context.Context, db *sql.DB, lo, hi int64) error {
	// Begin the transaction.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert both bounds and commit.
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO "+metaTable+" (name, value) VALUES (?, ?) "+ //nolint:gosec // the table name is the cache's constant.
			"ON CONFLICT (name) DO UPDATE SET value = excluded.value",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for name, value := range map[string]int64{metaStartLo: lo, metaStartHi: hi} {
		if _, err := stmt.ExecContext(ctx, name, value); err != nil {
			return err
		}
	}
	return tx.Commit()
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
