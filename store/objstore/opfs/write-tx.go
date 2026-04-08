//go:build js

package store_objstore_opfs

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/pkg/errors"
)

// writeTx is a write-only transaction that applies mutations with per-file
// WebLock coordination. Created by the txcache's newWriteTx callback during
// commit. The txcache calls Set/Delete for each buffered mutation, then
// calls Commit.
type writeTx struct {
	store   *Store
	release func() // exclusive WebLock release
	once    sync.Once
}

// newWriteTx acquires an exclusive WebLock and returns a write-only tx.
func newWriteTx(s *Store) (*writeTx, error) {
	release, err := filelock.AcquireWebLock(s.lockName, true)
	if err != nil {
		return nil, errors.Wrap(err, "acquire exclusive WebLock")
	}
	return &writeTx{store: s, release: release}, nil
}

// Set writes a key-value pair with per-file locking.
func (t *writeTx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	shard := shardPrefix(encoded)

	shardDir, err := getShardDir(t.store.root, shard, true)
	if err != nil {
		return errors.Wrap(err, "create shard dir")
	}

	file, rel, err := filelock.AcquireFile(shardDir, encoded, t.store.lockPrefix+"/"+shard, true)
	if err != nil {
		return errors.Wrap(err, "acquire file")
	}
	defer rel()

	file.Truncate(0)
	if _, err := file.WriteAt(value, 0); err != nil {
		return errors.Wrap(err, "write")
	}
	file.Flush()
	return nil
}

// Delete removes a key.
func (t *writeTx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	shard := shardPrefix(encoded)

	shardDir, err := getShardDir(t.store.root, shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil
		}
		return err
	}

	err = opfs.DeleteFile(shardDir, encoded)
	if opfs.IsNotFound(err) {
		return nil
	}
	return err
}

// Commit finalizes the write transaction. Mutations were already applied
// by Set/Delete calls; this just releases the exclusive WebLock.
func (t *writeTx) Commit(ctx context.Context) error {
	t.once.Do(func() { t.release() })
	return nil
}

// Discard cancels the transaction and releases the exclusive WebLock.
func (t *writeTx) Discard() {
	t.once.Do(func() { t.release() })
}

// The following methods satisfy kvtx.Tx but are never called on the write tx.
// The txcache only calls Set, Delete, Commit, and Discard.

// Get is not supported on write-only transactions.
func (t *writeTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	return nil, false, errors.New("write-only tx")
}

// Exists is not supported on write-only transactions.
func (t *writeTx) Exists(ctx context.Context, key []byte) (bool, error) {
	return false, errors.New("write-only tx")
}

// Size is not supported on write-only transactions.
func (t *writeTx) Size(ctx context.Context) (uint64, error) {
	return 0, errors.New("write-only tx")
}

// ScanPrefix is not supported on write-only transactions.
func (t *writeTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return errors.New("write-only tx")
}

// ScanPrefixKeys is not supported on write-only transactions.
func (t *writeTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return errors.New("write-only tx")
}

// Iterate is not supported on write-only transactions.
func (t *writeTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx.NewErrIterator(errors.New("write-only tx"))
}

// _ is a type assertion.
var _ kvtx.Tx = (*writeTx)(nil)
