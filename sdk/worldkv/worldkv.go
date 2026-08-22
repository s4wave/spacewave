// Package worldkv provides a high-level world-backed key/value store.
//
// It wraps the lower-level world-backed KVTX store with create-or-open
// object lifecycle, transaction helpers, JSON conveniences, prefix scans,
// and live watch subscriptions. It serves both the Spacewave SDK surface
// and standalone sync-engine consumers.
package worldkv

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	"github.com/sirupsen/logrus"
)

// backing is the minimal contract the sugar layer needs: transactions,
// live prefix snapshots, and an optional teardown.
type backing interface {
	kvtx.Store
	kvtx.WatchStore
}

// closeable is implemented by backing stores that hold resources.
type closeable interface {
	Close()
}

// Store is a high-level world-backed key/value store.
//
// Values are arbitrary bytes. All mutations are transactional and commit
// through world operations, so they flow through the same validation and
// watch paths as every other world mutation. The backing may be local
// (world object) or remote (kvtx RPC), hosted or embedded.
type Store struct {
	le    *logrus.Entry
	ws    world.WorldState
	key   string
	inner backing
}

// Open opens or creates the key/value store object at objectKey within the
// world state. The returned Store owns the backing object cursor; call
// Close when done.
func Open(ctx context.Context, le *logrus.Entry, ws world.WorldState, objectKey string) (*Store, error) {
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Create the backing object with an empty key/value workload block if
	// it does not exist yet.
	_, exists, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "worldkv: look up object")
	}
	if !exists {
		if _, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
			return nil
		}); err != nil {
			return nil, errors.Wrap(err, "worldkv: create object")
		}
	}
	// Stamp the object type only when missing so repeated opens stay cheap.
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
		if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
			return nil, errors.Wrap(err, "worldkv: set object type")
		}
	}

	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "worldkv: open object")
	}
	var inner *s4wave_kv_world.WorldBackedStore
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		inner, err = s4wave_kv_world.NewWorldBackedStore(ctx, le, root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		return nil, errors.Wrap(err, "worldkv: open backing store")
	}
	if inner == nil {
		return nil, errors.New("worldkv: failed to open backing store")
	}
	return &Store{le: le, ws: ws, key: objectKey, inner: inner}, nil
}

// OpenRemote wraps an existing backing store - for example a kvtx RPC
// client store connected over WebSocket - with the same sugar surface.
// The caller retains ownership of the backing store.
func OpenRemote(ctx context.Context, le *logrus.Entry, store backing) (*Store, error) {
	if store == nil {
		return nil, errors.New("worldkv: backing store is required")
	}
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	return &Store{le: le, inner: store}, nil
}

// Close releases the backing store when it owns resources.
func (s *Store) Close() {
	if c, ok := s.inner.(closeable); ok {
		c.Close()
	}
}

// Update runs fn in a write transaction, committing on success.
func (s *Store) Update(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.inner.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Discard()
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		return err
	}
	tx.Discard()
	return nil
}

// View runs fn in a read-only transaction.
func (s *Store) View(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.inner.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// Put sets a single key.
func (s *Store) Put(ctx context.Context, key string, value []byte) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(key), value)
	})
}

// PutMany sets every entry in a single transaction.
func (s *Store) PutMany(ctx context.Context, entries map[string][]byte) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		for key, value := range entries {
			if err := tx.Set(ctx, []byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Get returns the value at key.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var data []byte
	var found bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		data, found, err = tx.Get(ctx, []byte(key))
		return err
	})
	return data, found, err
}

// GetJSON unmarshals the JSON value at key into out. Returns

// Delete deletes a single key. Not found is not an error.
func (s *Store) Delete(ctx context.Context, key string) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Delete(ctx, []byte(key))
	})
}

// List returns the key/value pairs under prefix, ordered by the scan.
func (s *Store) List(ctx context.Context, prefix string) ([]kvtx.WatchEntry, error) {
	var out []kvtx.WatchEntry
	err := s.View(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(prefix), func(key, value []byte) error {
			out = append(out, kvtx.WatchEntry{Key: append([]byte(nil), key...), Value: append([]byte(nil), value...)})
			return nil
		})
	})
	return out, err
}

// Watch subscribes to changes under prefix. cb receives the current
// snapshot immediately and each changed snapshot after commits. The
// returned cancel function stops the subscription. cb must not block.
func (s *Store) Watch(ctx context.Context, prefix string, cb func(entries []kvtx.WatchEntry)) (func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		err := s.inner.WatchPrefix(watchCtx, []byte(prefix), func(entries []kvtx.WatchEntry) error {
			cp := make([]kvtx.WatchEntry, 0, len(entries))
			for _, e := range entries {
				cp = append(cp, kvtx.WatchEntry{Key: append([]byte(nil), e.Key...), Value: append([]byte(nil), e.Value...)})
			}
			cb(cp)
			return nil
		})
		done <- err
	}()
	return func() {
		cancel()
		<-done
	}, nil
}

// Key returns the backing object key.
func (s *Store) Key() string {
	return s.key
}

// WatchPrefix subscribes to snapshot changes under prefix. cb receives the
// current snapshot immediately and each changed snapshot after commits,
// cloned so callers may retain it. Implements kvtx.WatchStore.
func (s *Store) WatchPrefix(ctx context.Context, prefix string, cb func(entries []kvtx.WatchEntry) error) error {
	if s == nil || s.inner == nil {
		return errors.New("worldkv: store is closed")
	}
	return s.inner.WatchPrefix(ctx, []byte(prefix), func(entries []kvtx.WatchEntry) error {
		cp := make([]kvtx.WatchEntry, 0, len(entries))
		for _, e := range entries {
			cp = append(cp, kvtx.WatchEntry{Key: append([]byte(nil), e.Key...), Value: append([]byte(nil), e.Value...)})
		}
		return cb(cp)
	})
}

// Exists reports whether key exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		exists, err = tx.Exists(ctx, []byte(key))
		return err
	})
	return exists, err
}
