package main

import (
	"context"
	"path/filepath"

	bbolt "github.com/aperturerobotics/bbolt"
	bdb "github.com/dgraph-io/badger/v4"
	"github.com/pkg/errors"
	store_kvtx_badger "github.com/s4wave/spacewave/db/store/kvtx/badger"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
)

// engines lists the engines the harness can drive.
var engines = []engine{
	{name: "bolt", open: openBolt},
	{name: "badger", open: openBadger},
	{name: "logindex", open: openLogindex},
}

// openBolt opens a bbolt file with the bucket the volume uses.
func openBolt(ctx context.Context, dir string) (store, error) {
	return store_kvtx_bolt.Open(filepath.Join(dir, "bolt.db"), 0o600, &bbolt.Options{NoFreelistSync: true}, []byte("hydra"))
}

// badgerStore closes the badger database under the kvtx store.
type badgerStore struct {
	*store_kvtx_badger.Store
}

// Close closes the badger database.
func (s badgerStore) Close() error {
	return s.GetDB().Close()
}

// Sync makes ordered commits durable.
func (s badgerStore) Sync(context.Context) error {
	return s.GetDB().Sync()
}

// Compact runs value log garbage collection until it finds nothing to collect.
func (s badgerStore) Compact() error {
	for {
		err := s.GetDB().RunValueLogGC(0.5)
		if errors.Is(err, bdb.ErrNoRewrite) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// openBadger opens badger with the volume's defaults.
func openBadger(ctx context.Context, dir string) (store, error) {
	// Match the volume's options: durable writes, no conflict detection.
	o := bdb.DefaultOptions(dir)
	o.DetectConflicts = false
	o.SyncWrites = true
	o.Logger = nil
	s, err := store_kvtx_badger.Open(o)
	if err != nil {
		return nil, err
	}
	return badgerStore{s}, nil
}

// logindexStore closes the device under the index.
type logindexStore struct {
	*logindex.Index
}

// openLogindex opens the in-memory copy-on-write table with its log and
// checkpoint on a directory device. It holds every value in memory.
func openLogindex(ctx context.Context, dir string) (store, error) {
	// Open the device, then the index on it.
	dev, err := device.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	idx, err := logindex.Open(ctx, dev, logindex.Options{})
	if err != nil {
		return nil, err
	}
	return logindexStore{idx}, nil
}
