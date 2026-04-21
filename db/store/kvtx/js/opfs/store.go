//go:build js

package store_kvtx_opfs

import (
	"context"
	"sync"
	"syscall/js"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	"github.com/s4wave/spacewave/db/opfs"
)

// Store is an OPFS-backed key-value store using WebLock coordination.
// Keys are hex-encoded for filesystem-safe filenames with 2-char prefix sharding.
type Store struct {
	root     js.Value // OPFS directory handle for this store
	lockName string   // WebLock key (e.g. "<volume-id>|<store-id>")
	sync     bool     // use sync access handles (DedicatedWorker only)
}

// NewStore constructs a new OPFS key-value store.
// root is the OPFS directory handle for this store's data.
// lockName is the WebLock key used for transaction coordination.
// Automatically detects and prefers sync access handles when available.
func NewStore(root js.Value, lockName string) *Store {
	return &Store{root: root, lockName: lockName, sync: opfs.SyncAvailable()}
}

// NewTransaction returns a new transaction against the store.
// Write transactions are wrapped with txcache: reads use a shared lock,
// writes buffer in memory, and the exclusive lock is only acquired at Commit.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	readTx, err := s.newRawTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	if !write {
		return readTx, nil
	}
	// Eagerly clean up stale .pending marker from a crashed write.
	// Safe without exclusive lock: an in-progress commit does not
	// re-check the marker, and deletion is idempotent.
	_ = opfs.DeleteFile(s.root, pendingMarker)
	return kvtx_txcache.NewTxWithCbs(readTx, true, readTx.Discard, func() (kvtx.Tx, error) {
		return s.newRawTransaction(ctx, true)
	}, true)
}

// newRawTransaction creates a raw transaction with direct WebLock acquisition.
func (s *Store) newRawTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	release, err := acquireWebLock(s.lockName, write)
	if err != nil {
		return nil, errors.Wrap(err, "acquire WebLock")
	}

	tx := &Tx{
		store:   s,
		write:   write,
		release: release,
	}
	if write {
		tx.sets = make(map[string][]byte)
		tx.deletes = make(map[string]struct{})

		// Crash recovery: check for a pending marker from a previous crashed write.
		if err := tx.cleanupPending(); err != nil {
			release()
			return nil, errors.Wrap(err, "cleanup pending")
		}
	}
	return tx, nil
}

// Execute is a no-op for OPFS stores.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// encodeKey b58-encodes a key for use as an OPFS filename.
func encodeKey(key []byte) string {
	return b58.FastBase58Encoding(key)
}

// decodeKey b58-decodes an OPFS filename back to a key.
func decodeKey(encoded string) ([]byte, error) {
	return b58.FastBase58Decoding(encoded)
}

// shardPrefix returns the 2-char shard directory name for an encoded key.
func shardPrefix(encoded string) string {
	if len(encoded) < 2 {
		return "00"
	}
	return encoded[:2]
}

// acquireWebLock acquires a WebLock with the given name.
// If exclusive is true, acquires an exclusive lock; otherwise shared.
// Returns a release function that must be called to release the lock.
func acquireWebLock(name string, exclusive bool) (func(), error) {
	acquiredCh := make(chan struct{})
	var resolveFunc js.Value

	mode := "shared"
	if exclusive {
		mode = "exclusive"
	}

	var executorCb js.Func
	lockCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		executorCb = js.FuncOf(func(this js.Value, pArgs []js.Value) any {
			resolveFunc = pArgs[0]
			close(acquiredCh)
			return nil
		})
		return js.Global().Get("Promise").New(executorCb)
	})

	opts := js.Global().Get("Object").New()
	opts.Set("mode", mode)

	js.Global().Get("navigator").Get("locks").Call("request", name, opts, lockCb)
	<-acquiredCh

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			if resolveFunc.IsUndefined() || resolveFunc.IsNull() || resolveFunc.Type() != js.TypeFunction {
				panic("opfs store weblock release callback unavailable")
			}
			defer func() {
				if e := recover(); e != nil {
					panic("opfs store weblock release invoke failed")
				}
			}()
			resolveFunc.Invoke()
			executorCb.Release()
			lockCb.Release()
		})
	}, nil
}

// _ is a type assertion.
var _ kvtx.Store = ((*Store)(nil))
