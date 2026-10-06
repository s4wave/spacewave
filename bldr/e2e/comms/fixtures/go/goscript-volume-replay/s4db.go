//go:build goscript

package goscript_volume_replay

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume/device"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	"github.com/s4wave/spacewave/db/volume/direct"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
	"github.com/s4wave/spacewave/db/volume/records"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// s4dbName is the device file holding the s4db database.
const s4dbName = "db.s4wave"

// s4dbRecords is a record store in an s4db database. Each Commit is one
// s4db commit, durable or ordered, so commits apply atomically and a crash
// keeps a prefix of the ordered ones.
type s4dbRecords struct {
	db *s4db.DB
}

// view runs fn in a read transaction.
func (r s4dbRecords) view(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := r.db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// Get reads the records of keys, nil for an absent key. s4db returns values
// the caller owns, so they need no copy.
func (r s4dbRecords) Get(ctx context.Context, keys [][]byte) ([][]byte, error) {
	out := make([][]byte, len(keys))
	err := r.view(ctx, func(tx kvtx.Tx) error {
		for i, k := range keys {
			v, found, err := tx.Get(ctx, k)
			if err != nil {
				return err
			}
			if found {
				out[i] = v
			}
		}
		return nil
	})
	return out, err
}

// Has reports which keys have records.
func (r s4dbRecords) Has(ctx context.Context, keys [][]byte) ([]bool, error) {
	out := make([]bool, len(keys))
	err := r.view(ctx, func(tx kvtx.Tx) error {
		for i, k := range keys {
			found, err := tx.Exists(ctx, k)
			if err != nil {
				return err
			}
			out[i] = found
		}
		return nil
	})
	return out, err
}

// Scan calls fn with a copy of each record whose key has prefix, in key
// order.
func (r s4dbRecords) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	return r.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
			return fn(bytes.Clone(k), bytes.Clone(v))
		})
	})
}

// Commit applies ops in one commit, durable if durable is set.
func (r s4dbRecords) Commit(ctx context.Context, ops []records.Op, durable bool) error {
	// Stage the ops in a write transaction.
	tx, err := r.db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, op := range ops {
		if op.Delete {
			err = tx.Delete(ctx, op.Key)
		} else {
			err = tx.Set(ctx, op.Key, op.Value)
		}
		if err != nil {
			return err
		}
	}

	// Commit durably or with write ordering only.
	if durable {
		return tx.Commit(ctx)
	}
	return kvtx.CommitOrdered(ctx, tx)
}

// openS4DB opens the direct engine over s4db on a device from open, which
// reopens the same storage.
func openS4DB(ctx context.Context, open func() (device.Device, func() error, error), destroy func() error) (engine, error) {
	// start opens the device, the database, and the engine.
	c := &counter{}
	start := func() (*direct.Store, func() error, error) {
		// Open the device and the database in it.
		dev, closeDevice, err := open()
		if err != nil {
			return nil, nil, err
		}
		db, err := s4db.OpenDevice(ctx, countingDevice{Device: dev, counter: c}, s4dbName, s4db.Options{})
		if err != nil {
			_ = closeDevice()
			return nil, nil, err
		}

		// Open the engine on the database.
		s, err := direct.Open(ctx, s4dbRecords{db: db})
		if err != nil {
			_ = db.Close()
			_ = closeDevice()
			return nil, nil, err
		}
		return s, func() error {
			_ = s.Close()
			_ = db.Close()
			return closeDevice()
		}, nil
	}

	// Open the engine and reopen it on the same storage on request.
	s, closeStore, err := start()
	if err != nil {
		return engine{}, err
	}
	return engine{
		target: s,
		reopen: func(ctx context.Context) (workload.Target, error) {
			if err := closeStore(); err != nil {
				return nil, err
			}
			s, closeStore, err = start()
			if err != nil {
				closeStore = func() error { return nil }
				return nil, err
			}
			return s, nil
		},
		counts: c.snapshot,
		destroy: func() error {
			_ = closeStore()
			return destroy()
		},
	}, nil
}

// openS4DBOPFS opens s4db on a fresh OPFS device.
func openS4DBOPFS(ctx context.Context) (engine, error) {
	path := storageName + "/s4db"
	if err := device_opfs.Delete(path); err != nil {
		return engine{}, err
	}
	return openS4DB(ctx, func() (device.Device, func() error, error) {
		d, err := device_opfs.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}, func() error {
		return device_opfs.Delete(storageName)
	})
}

// openS4DBIDB opens s4db on a fresh IndexedDB device.
func openS4DBIDB(ctx context.Context) (engine, error) {
	name := storageName + "-s4db"
	if err := volume_idb.DeleteDatabase(name); err != nil {
		return engine{}, err
	}
	return openS4DB(ctx, func() (device.Device, func() error, error) {
		d, err := volume_idb.OpenDevice(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}, func() error {
		return volume_idb.DeleteDatabase(name)
	})
}

// _ is a type assertion
var _ records.Store = s4dbRecords{}
