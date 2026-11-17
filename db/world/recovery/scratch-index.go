//go:build !js && !wasip1

package world_recovery

import (
	"context"
	"errors"
	"os"

	"github.com/s4wave/spacewave/db/block"
	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// scratchIndex composes fixed-budget SQLite transactions with named disk indexes.
// The SQLite pager, connection count, mmap and temp storage are bounded at open.
// No in-memory freelist, frontier or all-record index is rebuilt on resume.
//
// Successive Updates share one open write transaction, the batch, until
// Commit; View reads inside it, so a step sees every earlier step's rows.
// Every step saves the whole result beside its rows, so each prefix of steps
// is a consistent state, and a crash or Discard returns to the last Commit,
// which a resumed run continues exactly as a fresh run would.
type scratchIndex struct {
	// store owns the existing SQLite KV primitive and its durability barrier.
	store sqlite.Store
	// batch is the open write transaction the Updates since the last Commit
	// share, nil when no step is pending.
	batch *scratchTx
	// steps counts the Updates the batch holds.
	steps int
}

// batchSteps and batchBytes bound one batch, so a crash repeats at most this
// much work and the WAL grows by about one batch before a checkpoint.
const (
	batchSteps = 4096
	batchBytes = 8 << 20
)

// openScratch opens one table for all scratch indexes, with a fixed pager budget.
func openScratch(path string, readOnly bool) (*scratchIndex, error) {
	// Resolve read-only report paths as well as the already validated count path.
	canonical, err := canonicalScratchPath(path)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		file, err := os.OpenFile(canonical, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			err = file.Close()
		} else if os.IsExist(err) {
			err = nil
		}
		if err != nil {
			return nil, err
		}
	}

	// Configure every connection before opening any read or write transaction.
	store, err := sqlite.OpenWithPragmas(context.Background(), canonical, "recovery", sqlite.Pragmas{FullSync: true, CacheSize: -2048, DisableMmap: true, TempStore: 1, ReadOnly: readOnly, WALCheckpointBytes: 8 << 20})
	if err != nil {
		return nil, err
	}
	store.GetDB().SetMaxOpenConns(4)
	store.GetDB().SetMaxIdleConns(4)
	return &scratchIndex{store: store}, nil
}

// View runs one bounded snapshot, inside the batch when one is open, and
// retains every failed indexed read.
func (db *scratchIndex) View(fn func(*scratchTx) error) error {
	// Read the open batch's uncommitted rows with their own error scope.
	if db.batch != nil {
		scope := &scratchTx{tx: db.batch.tx, ctx: db.batch.ctx}
		return errors.Join(fn(scope), scope.err)
	}

	// Otherwise read one committed snapshot, as background work.
	ctx := block.WithBackground(context.Background())
	tx, err := db.store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	scope := &scratchTx{tx: tx, ctx: ctx}
	return errors.Join(fn(scope), scope.err)
}

// Update adds one step's rows, cursor and aggregate to the batch. A failed
// step discards the whole batch, so no partial step can ever commit.
func (db *scratchIndex) Update(fn func(*scratchTx) error) error {
	// Keep cancellation of the scan separate from persisting its last durable
	// report, and queue the batch behind foreground writers as background work.
	if db.batch == nil {
		ctx := block.WithBackground(context.Background())
		tx, err := db.store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		db.batch = &scratchTx{tx: tx, ctx: ctx}
	}
	if err := errors.Join(fn(db.batch), db.batch.err); err != nil {
		db.Discard()
		return err
	}
	db.steps++
	return nil
}

// Full reports whether the batch reached its step or byte bound.
func (db *scratchIndex) Full() bool {
	return db.batch != nil && (db.steps >= batchSteps || db.batch.written >= batchBytes)
}

// Commit makes the batch durable as one atomic transaction.
func (db *scratchIndex) Commit() error {
	// An empty batch has nothing to make durable.
	if db.batch == nil {
		return nil
	}

	// Clear the batch before committing, so a failed commit is not retried.
	batch := db.batch
	db.batch, db.steps = nil, 0
	defer batch.tx.Discard()
	return batch.tx.Commit(batch.ctx)
}

// Discard rolls the batch back to the last Commit.
func (db *scratchIndex) Discard() {
	if db.batch != nil {
		db.batch.tx.Discard()
		db.batch, db.steps = nil, 0
	}
}

// Close stops only the scratch primitive's local durability/checkpoint machinery.
func (db *scratchIndex) Close() error {
	db.Discard()
	return db.store.Close()
}
