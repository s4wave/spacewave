package block_store_fillcache

import (
	"context"
	"crypto/rand"
	"database/sql"
	"math"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// evictChunkBytes is the block bytes one eviction step deletes, so a reader or
// a fill queued behind the step waits for a bounded write.
const evictChunkBytes = 4 << 20

// evictChunkRows bounds the rows one eviction step scans for unstamped rows,
// and the rows it reads in sequence order.
const evictChunkRows = 4096

// evictPause is the time between two eviction steps when no fill waits, which
// paces eviction below the disk's speed.
const evictPause = 100 * time.Millisecond

// evictLowWater is the fraction of the size bound, in percent, that eviction
// stops at once it has started.
const evictLowWater = 90

// evicter keeps the live bytes of the cache's table under a bound by deleting
// the rows read longest ago, as readsTable orders them, sparing the protected
// start range until nothing else is left. Each step first deletes the
// unstamped rows among a run of adjacent keys from a random key, which block
// hashes make a uniform sample, then the rows with the smallest read sequence
// below the range, then those above it, and only when both are exhausted the
// oldest inside it, until it has deleted a chunk. A range that fills the bound
// therefore still yields rows, and eviction never stalls.
type evicter struct {
	// db is the cache's database.
	db *sql.DB
	// table is the key/value table.
	table string
	// keys builds the table's block keys.
	keys *store_kvkey.KVKey
	// limit is the live byte size past which eviction starts.
	limit int64
	// target is the live byte size at which eviction stops.
	target int64
	// chunk is the block bytes one step deletes: evictChunkBytes, or a tenth of
	// the limit when that is smaller, so a step never empties a small cache.
	chunk int64
	// evicted counts the rows the steps deleted.
	evicted *atomic.Uint64
	// lo and hi bound the protected start range, inclusive. With no range
	// both are math.MaxInt64, so every stamp falls below it. Only the writer
	// uses them.
	lo, hi int64
}

// seqRange is a range of read sequences that one eviction query reads in
// sequence order, through the index on the reads table's sequence.
type seqRange struct {
	// where is the condition on r.seq.
	where string
	// args are the condition's arguments.
	args []any
}

// newEvicter builds an evicter for the table with the byte bound, counting the
// rows it deletes in evicted.
func newEvicter(db *sql.DB, table string, keys *store_kvkey.KVKey, limit int64, evicted *atomic.Uint64) *evicter {
	return &evicter{
		db:      db,
		table:   table,
		keys:    keys,
		limit:   limit,
		target:  limit / 100 * evictLowWater,
		chunk:   min(evictChunkBytes, limit/10),
		evicted: evicted,
		lo:      math.MaxInt64,
		hi:      math.MaxInt64,
	}
}

// protect sets the protected start range to lo through hi inclusive.
func (e *evicter) protect(lo, hi int64) {
	e.lo, e.hi = lo, hi
}

// order returns the stamped ranges in the order a step deletes from them:
// below the protected range, above it, then inside it.
func (e *evicter) order() []seqRange {
	return []seqRange{
		{where: "r.seq < ?", args: []any{e.lo}},
		{where: "r.seq > ?", args: []any{e.hi}},
		{where: "r.seq BETWEEN ? AND ?", args: []any{e.lo, e.hi}},
	}
}

// used returns the bytes of the database file that hold rows: its pages less
// the free ones. Deleting rows frees pages for new rows without shrinking the
// file, so the file size is not the cache's size.
func (e *evicter) used(ctx context.Context) (int64, error) {
	var used int64
	err := e.db.QueryRowContext(ctx,
		"SELECT (page_count - freelist_count) * page_size FROM pragma_page_count(), pragma_freelist_count(), pragma_page_size()",
	).Scan(&used)
	return used, err
}

// next decides whether the cache is evicting after the step it takes now.
// Outside an eviction it starts one when the cache is over its limit; during
// one it deletes a chunk of rows and goes on until the cache is under its
// target.
func (e *evicter) next(ctx context.Context, evicting bool) (bool, error) {
	// Measure the cache and decide whether it needs a step.
	used, err := e.used(ctx)
	if err != nil {
		return evicting, errors.Wrap(err, "measure the fill cache")
	}
	if used <= e.target || (!evicting && used <= e.limit) {
		return false, nil
	}

	// Delete a chunk of rows.
	if err := e.step(ctx); err != nil {
		return true, errors.Wrap(err, "evict from the fill cache")
	}
	return true, nil
}

// step deletes about a chunk of rows: the unstamped rows of a sampled run,
// then the rows read longest ago, in the ranges order gives. A step moves to
// the next range only when the one before is exhausted.
func (e *evicter) step(ctx context.Context) error {
	// Take the unstamped rows of a run from a random key.
	start, err := e.randomKey()
	if err != nil {
		return err
	}
	keys, size, err := e.unstamped(ctx, start)
	if err != nil {
		return err
	}

	// Fill the chunk with the rows read longest ago, range by range.
	for _, r := range e.order() {
		if size >= e.chunk {
			break
		}
		oldest, n, more, err := e.oldest(ctx, r, e.chunk-size)
		if err != nil {
			return err
		}
		keys = append(keys, oldest...)
		size += n
		if more {
			break
		}
	}
	return e.delete(ctx, keys)
}

// unstamped returns the keys and summed size of the rows with no read stamp
// among the evictChunkRows rows at or after start, up to a chunk.
func (e *evicter) unstamped(ctx context.Context, start []byte) ([][]byte, int64, error) {
	// Query the run's rows that have no stamp.
	run := "SELECT key, length(value) AS size FROM " + e.table + " WHERE key >= ? ORDER BY key LIMIT ?"
	rows, err := e.db.QueryContext(ctx,
		"SELECT c.key, c.size FROM ("+run+") c "+ //nolint:gosec // the table names are the cache's constants.
			"WHERE NOT EXISTS (SELECT 1 FROM "+readsTable+" r WHERE r.key = c.key)",
		start, evictChunkRows,
	)
	if err != nil {
		return nil, 0, err
	}

	// Read them up to a chunk.
	keys, size, _, err := e.collect(rows, e.chunk)
	return keys, size, err
}

// oldest returns the keys and summed size of the rows in r read longest ago,
// up to need bytes, and whether r may hold more: it read evictChunkRows rows
// or reached need. A stamp whose row is gone counts no bytes and is returned
// for deletion.
func (e *evicter) oldest(ctx context.Context, r seqRange, need int64) ([][]byte, int64, bool, error) {
	// Query the range's stamps in sequence order with their rows' sizes.
	rows, err := e.db.QueryContext(ctx,
		"SELECT r.key, COALESCE(length(c.value), 0) FROM "+readsTable+" r "+ //nolint:gosec // the table names and conditions are the cache's constants.
			"LEFT JOIN "+e.table+" c ON c.key = r.key WHERE "+r.where+" ORDER BY r.seq LIMIT ?",
		append(r.args, evictChunkRows)...,
	)
	if err != nil {
		return nil, 0, false, err
	}

	// Read them up to need, noting whether the range may hold more.
	keys, size, n, err := e.collect(rows, need)
	return keys, size, size >= need || n == evictChunkRows, err
}

// collect reads key and size rows until their sizes reach limit, closes rows,
// and returns the keys, their summed size and how many rows it read.
func (e *evicter) collect(rows *sql.Rows, limit int64) ([][]byte, int64, int, error) {
	// Read rows until their sizes reach the limit.
	defer rows.Close()
	var keys [][]byte
	var size int64
	for size < limit && rows.Next() {
		var key []byte
		var n int64
		if err := rows.Scan(&key, &n); err != nil {
			return nil, 0, 0, err
		}
		keys = append(keys, key)
		size += n
	}
	return keys, size, len(keys), errors.Wrap(rows.Err(), "scan the fill cache")
}

// delete deletes the rows and read stamps of keys in one transaction and
// counts the rows.
func (e *evicter) delete(ctx context.Context, keys [][]byte) error {
	// Skip an empty chunk.
	if len(keys) == 0 {
		return nil
	}

	// Begin the transaction.
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Prepare the row and stamp deletes.
	delRow, err := tx.PrepareContext(ctx, "DELETE FROM "+e.table+" WHERE key = ?") //nolint:gosec // the table name is the cache's constant.
	if err != nil {
		return err
	}
	defer delRow.Close()
	delStamp, err := tx.PrepareContext(ctx, "DELETE FROM "+readsTable+" WHERE key = ?") //nolint:gosec // the table name is the cache's constant.
	if err != nil {
		return err
	}
	defer delStamp.Close()

	// Delete each key's row and stamp, counting the rows.
	var n int64
	for _, key := range keys {
		res, err := delRow.ExecContext(ctx, key)
		if err != nil {
			return err
		}
		if m, err := res.RowsAffected(); err == nil {
			n += m
		}
		if _, err := delStamp.ExecContext(ctx, key); err != nil {
			return err
		}
	}

	// Commit and count the deleted rows.
	if err := tx.Commit(); err != nil {
		return err
	}
	e.evicted.Add(uint64(n))
	return nil
}

// blockKey returns the table key of the block ref, as the block store builds
// it.
func (e *evicter) blockKey(ref *block.BlockRef) ([]byte, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	return e.keys.GetBlockKey(rm), nil
}

// randomKey returns a block key with a random digest, built the way the cache
// builds the key of any block, so it falls among the keys of the table.
func (e *evicter) randomKey() ([]byte, error) {
	// Hash random bytes into a block reference of the default hash type.
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return nil, err
	}
	ref, err := block.BuildBlockRef(data, &block.PutOpts{})
	if err != nil {
		return nil, err
	}
	return e.blockKey(ref)
}
