//go:build !js && !wasip1

package world_recovery

import (
	"context"
	"errors"
	"os"

	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// scratchIndex composes fixed-budget SQLite transactions with named disk indexes.
// The SQLite pager, connection count, mmap and temp storage are bounded at open.
// No in-memory freelist, frontier or all-record index is rebuilt on resume.
type scratchIndex struct {
	// store owns the existing SQLite KV primitive and its durability barrier.
	store sqlite.Store
}

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

// View runs one bounded snapshot and retains every failed indexed read.
func (db *scratchIndex) View(fn func(*scratchTx) error) error {
	return db.transaction(false, fn)
}

// Update atomically commits the rows, cursor and aggregate or rolls all of them back.
func (db *scratchIndex) Update(fn func(*scratchTx) error) error {
	return db.transaction(true, fn)
}

// transaction preserves SQLite errors even for index lookups whose caller expects absence.
func (db *scratchIndex) transaction(write bool, fn func(*scratchTx) error) error {
	// Keep cancellation of the scan separate from persisting its last durable report.
	ctx := context.Background()
	tx, err := db.store.NewTransaction(ctx, write)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Commit only after the callback and every indexed lookup have succeeded.
	scope := &scratchTx{tx: tx, ctx: ctx}
	callbackErr := fn(scope)
	if err := errors.Join(callbackErr, scope.err); err != nil {
		return err
	}
	if write {
		return tx.Commit(ctx)
	}
	return nil
}

// Close stops only the scratch primitive's local durability/checkpoint machinery.
func (db *scratchIndex) Close() error {
	return db.store.Close()
}
