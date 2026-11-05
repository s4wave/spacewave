package block_store_fillcache

import (
	"context"
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// evictChunkBytes is the block bytes one eviction step deletes, so a reader or
// a fill queued behind the step waits for a bounded write.
const evictChunkBytes = 4 << 20

// evictChunkRows bounds the rows one eviction step scans.
const evictChunkRows = 4096

// evictPause is the time between two eviction steps when no fill waits, which
// paces eviction below the disk's speed.
const evictPause = 100 * time.Millisecond

// evictLowWater is the fraction of the size bound, in percent, that eviction
// stops at once it has started.
const evictLowWater = 90

// evicter keeps the live bytes of the cache's table under a bound by deleting
// runs of rows. Block keys are hashes, so a run of adjacent keys is a uniform
// sample of the cache and the run's pages free whole.
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
}

// newEvicter builds an evicter for the table with the byte bound.
func newEvicter(db *sql.DB, table string, keys *store_kvkey.KVKey, limit int64) *evicter {
	return &evicter{
		db:     db,
		table:  table,
		keys:   keys,
		limit:  limit,
		target: limit / 100 * evictLowWater,
		chunk:  min(evictChunkBytes, limit/10),
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
// one it deletes a run of rows and goes on until the cache is under its target.
func (e *evicter) next(ctx context.Context, evicting bool) (bool, error) {
	// Measure the cache and decide whether it needs a step.
	used, err := e.used(ctx)
	if err != nil {
		return evicting, errors.Wrap(err, "measure the fill cache")
	}
	if used <= e.target || (!evicting && used <= e.limit) {
		return false, nil
	}

	// Delete a run of rows.
	if err := e.step(ctx); err != nil {
		return true, errors.Wrap(err, "evict from the fill cache")
	}
	return true, nil
}

// step deletes about a chunk of adjacent rows, starting at the first
// row at or after a random key and wrapping to the first row.
func (e *evicter) step(ctx context.Context) error {
	// Find the run from a random start, or from the start of the table.
	start, err := e.randomKey()
	if err != nil {
		return err
	}
	first, last, err := e.run(ctx, start)
	if err != nil {
		return err
	}
	if first == nil {
		first, last, err = e.run(ctx, []byte{0})
		if err != nil || first == nil {
			return err
		}
	}

	// Delete the run in one statement.
	_, err = e.db.ExecContext(ctx, "DELETE FROM "+e.table+" WHERE key >= ? AND key <= ?", first, last)
	return err
}

// run returns the first and last key of the rows from start whose values sum
// to about the chunk. It returns nil keys when no row is at or after
// start.
func (e *evicter) run(ctx context.Context, start []byte) (first, last []byte, err error) {
	// Scan the rows from start in key order.
	rows, err := e.db.QueryContext(ctx,
		"SELECT key, length(value) FROM "+e.table+" WHERE key >= ? ORDER BY key LIMIT ?",
		start, evictChunkRows,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	// Extend the run until it holds enough bytes.
	var size int64
	for rows.Next() && size < e.chunk {
		var key []byte
		var n int64
		if err := rows.Scan(&key, &n); err != nil {
			return nil, nil, err
		}
		if first == nil {
			first = key
		}
		last = key
		size += n
	}
	return first, last, errors.Wrap(rows.Err(), "scan the fill cache")
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

	// Encode the reference as the block store does.
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	return e.keys.GetBlockKey(rm), nil
}
