package s4wave_kv_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx "github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// KVStore is the consumer-facing contract for key/value stores.
type KVStore interface {
	Put(ctx context.Context, key string, value []byte) error
	PutMany(ctx context.Context, entries map[string][]byte) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Delete(ctx context.Context, key string) error
	DeleteMany(ctx context.Context, keys []string) error
	List(ctx context.Context, prefix string) ([]kvtx.WatchEntry, error)
	Exists(ctx context.Context, key string) (bool, error)
}

// WatchKVStore extends KVStore with live watch subscriptions and teardown.
type WatchKVStore interface {
	KVStore
	Watch(ctx context.Context, prefix string, cb func(entries []kvtx.WatchEntry)) (func(), error)
	Close()
}

// ---- WorldBackedStore convenience methods ----

// Update runs fn in a write transaction, committing on success.
func (s *WorldBackedStore) Update(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.NewTransaction(ctx, true)
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
func (s *WorldBackedStore) View(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// Put sets a single key.
func (s *WorldBackedStore) Put(ctx context.Context, key string, value []byte) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(key), value)
	})
}

// PutMany sets every entry in a single transaction.
func (s *WorldBackedStore) PutMany(ctx context.Context, entries map[string][]byte) error {
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
func (s *WorldBackedStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var data []byte
	var found bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		data, found, err = tx.Get(ctx, []byte(key))
		return err
	})
	return data, found, err
}

// Delete removes a key. Not found is not an error.
func (s *WorldBackedStore) Delete(ctx context.Context, key string) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Delete(ctx, []byte(key))
	})
}

// DeleteMany removes every key in a single transaction.
func (s *WorldBackedStore) DeleteMany(ctx context.Context, keys []string) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		for _, key := range keys {
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				return err
			}
		}
		return nil
	})
}

// List returns the key/value pairs under prefix.
func (s *WorldBackedStore) List(ctx context.Context, prefix string) ([]kvtx.WatchEntry, error) {
	var out []kvtx.WatchEntry
	err := s.View(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(prefix), func(key, value []byte) error {
			out = append(out, kvtx.WatchEntry{Key: append([]byte(nil), key...), Value: append([]byte(nil), value...)})
			return nil
		})
	})
	return out, err
}

// Exists reports whether key exists.
func (s *WorldBackedStore) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		exists, err = tx.Exists(ctx, []byte(key))
		return err
	})
	return exists, err
}

// Watch subscribes to changes under prefix.
func (s *WorldBackedStore) Watch(ctx context.Context, prefix string, cb func(entries []kvtx.WatchEntry)) (func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		err := s.WatchPrefix(watchCtx, []byte(prefix), func(entries []kvtx.WatchEntry) error {
			cb(entries)
			return nil
		})
		done <- err
	}()
	return func() { cancel(); <-done }, nil
}

// OpenOrCreate opens the key/value store object at objectKey within the
// world state, creating it if it does not exist.
func OpenOrCreate(ctx context.Context, le *logrus.Entry, ws world.WorldState, objectKey string) (*WorldBackedStore, error) {
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	_, exists, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "kv: look up object")
	}
	if !exists {
		if _, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
			return nil
		}); err != nil {
			return nil, errors.Wrap(err, "kv: create object")
		}
	}
	if err := world_types.CheckObjectType(ctx, ws, objectKey, KvStoreTypeID); err != nil {
		if err := world_types.SetObjectType(ctx, ws, objectKey, KvStoreTypeID); err != nil {
			return nil, errors.Wrap(err, "kv: set object type")
		}
	}

	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "kv: open object")
	}
	var store *WorldBackedStore
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = NewWorldBackedStore(ctx, le, root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		return nil, errors.Wrap(err, "kv: open backing store")
	}
	if store == nil {
		return nil, errors.New("kv: failed to open backing store")
	}
	return store, nil
}

// ---- RemoteStore for hosted mode ----

// RemoteStore provides key/value convenience methods over any KVTX-compatible
// backing such as a kvtx RPC client connected to a hosted world.
type RemoteStore struct {
	inner kvtx.Store
	ws    kvtx.WatchStore
}

// OpenRemote wraps an existing backing store with the convenience surface.
func OpenRemote(ctx context.Context, le *logrus.Entry, store interface {
	kvtx.Store
	kvtx.WatchStore
},
) (*RemoteStore, error) {
	if store == nil {
		return nil, errors.New("kv: backing store is required")
	}
	return &RemoteStore{inner: store, ws: store}, nil
}

// Close releases resources if the backing owns them.
func (s *RemoteStore) Close() {
	if c, ok := s.inner.(interface{ Close() }); ok {
		c.Close()
	}
}

// Update runs fn in a write transaction.
func (s *RemoteStore) Update(ctx context.Context, fn func(tx kvtx.Tx) error) error {
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
func (s *RemoteStore) View(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.inner.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// Put sets a single key.
func (s *RemoteStore) Put(ctx context.Context, key string, value []byte) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(key), value)
	})
}

// Get returns the value at key.
func (s *RemoteStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var data []byte
	var found bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		data, found, err = tx.Get(ctx, []byte(key))
		return err
	})
	return data, found, err
}

// List returns the key/value pairs under prefix.
func (s *RemoteStore) List(ctx context.Context, prefix string) ([]kvtx.WatchEntry, error) {
	var out []kvtx.WatchEntry
	err := s.View(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(prefix), func(key, value []byte) error {
			out = append(out, kvtx.WatchEntry{Key: append([]byte(nil), key...), Value: append([]byte(nil), value...)})
			return nil
		})
	})
	return out, err
}

// Watch subscribes to changes under prefix.
func (s *RemoteStore) Watch(ctx context.Context, prefix string, cb func(entries []kvtx.WatchEntry)) (func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		err := s.ws.WatchPrefix(watchCtx, []byte(prefix), func(entries []kvtx.WatchEntry) error {
			cb(entries)
			return nil
		})
		done <- err
	}()
	return func() { cancel(); <-done }, nil
}

// PutMany sets every entry in a single transaction.
func (s *RemoteStore) PutMany(ctx context.Context, entries map[string][]byte) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		for key, value := range entries {
			if err := tx.Set(ctx, []byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes a key.
func (s *RemoteStore) Delete(ctx context.Context, key string) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		return tx.Delete(ctx, []byte(key))
	})
}

// DeleteMany removes every key in a single transaction.
func (s *RemoteStore) DeleteMany(ctx context.Context, keys []string) error {
	return s.Update(ctx, func(tx kvtx.Tx) error {
		for _, key := range keys {
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Exists reports whether key exists.
func (s *RemoteStore) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.View(ctx, func(tx kvtx.Tx) error {
		var err error
		exists, err = tx.Exists(ctx, []byte(key))
		return err
	})
	return exists, err
}
