package s4db

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
)

// DB is an open database file.
type DB struct{}

// Open opens or creates the database file at path.
func Open(path string, opts Options) (*DB, error) {
	return nil, ErrUnimplemented
}

// OpenDevice opens or creates the database in the named file of dev.
func OpenDevice(ctx context.Context, dev device.Device, name string, opts Options) (*DB, error) {
	return nil, ErrUnimplemented
}

// NewTransaction returns a transaction on the latest commit.
func (db *DB) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return nil, ErrUnimplemented
}

// SupportsAtomicCommit reports that one transaction commits atomically.
func (db *DB) SupportsAtomicCommit() bool {
	return true
}

// WritersWaiting reports whether a write transaction waits to begin.
func (db *DB) WritersWaiting() bool {
	return false
}

// Sync makes every completed ordered commit durable.
func (db *DB) Sync(ctx context.Context) error {
	return ErrUnimplemented
}

// WaitDurable waits until every completed ordered commit is durable.
func (db *DB) WaitDurable(ctx context.Context) error {
	return ErrUnimplemented
}

// Seq returns the sequence number of the latest commit.
func (db *DB) Seq() uint64 {
	return 0
}

// WaitSeq waits until a commit after seq is visible.
func (db *DB) WaitSeq(ctx context.Context, seq uint64) error {
	return ErrUnimplemented
}

// Refresh loads the commits other processes made since the last one seen.
func (db *DB) Refresh(ctx context.Context) error {
	return ErrUnimplemented
}

// Shared reports whether another process has the file open.
func (db *DB) Shared() (bool, error) {
	return false, ErrUnimplemented
}

// Compact runs compaction until the file holds no reclaimable space.
func (db *DB) Compact(ctx context.Context) error {
	return ErrUnimplemented
}

// Release returns space freed by earlier commits to the disk.
func (db *DB) Release(ctx context.Context) error {
	return ErrUnimplemented
}

// Close closes the database.
func (db *DB) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ kvtx.OrderedCommitStore = (*DB)(nil)
	_ kvtx.AtomicCommitStore  = (*DB)(nil)
	_ kvtx.WriterQueueStore   = (*DB)(nil)
)
