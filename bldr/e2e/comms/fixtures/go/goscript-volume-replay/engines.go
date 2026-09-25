//go:build goscript

package goscript_volume_replay

import (
	"context"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/device"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	"github.com/s4wave/spacewave/db/volume/direct"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
	opfs_engine "github.com/s4wave/spacewave/db/volume/js/opfs/engine"
	"github.com/s4wave/spacewave/db/volume/logindex"
	"github.com/s4wave/spacewave/db/volume/paylog"
	"github.com/s4wave/spacewave/db/volume/records"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// counts counts the storage calls an engine makes.
type counts struct {
	// Writes counts device Write calls, record store Commit calls, or format
	// 3 backend Write and Remove calls.
	Writes int `json:"writes"`
	// Flushes counts flushed writes, durable commits, or format 3 backend
	// Write calls, each of which publishes a whole file.
	Flushes int `json:"flushes"`
	// Reads counts device Read calls, record store Get, Has, and Scan calls,
	// or format 3 backend Read calls.
	Reads int `json:"reads"`
	// Bytes counts written bytes.
	Bytes int64 `json:"bytes"`
}

// sub returns the counts in c made after base.
func (c counts) sub(base counts) counts {
	return counts{
		Writes:  c.Writes - base.Writes,
		Flushes: c.Flushes - base.Flushes,
		Reads:   c.Reads - base.Reads,
		Bytes:   c.Bytes - base.Bytes,
	}
}

// counter accumulates counts across goroutines.
type counter struct {
	// mtx guards c.
	mtx sync.Mutex
	// c holds the counts so far.
	c counts
}

// add adds one call.
func (c *counter) add(write, flush bool, n int64) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if !write {
		c.c.Reads++
		return
	}
	c.c.Writes++
	c.c.Bytes += n
	if flush {
		c.c.Flushes++
	}
}

// snapshot returns the counts so far.
func (c *counter) snapshot() counts {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.c
}

// countingDevice counts the calls made through a device.
type countingDevice struct {
	device.Device
	// counter holds the counts.
	counter *counter
}

// Write counts and forwards a write.
func (d countingDevice) Write(ctx context.Context, writes []device.Write, flush bool) error {
	var n int64
	for _, w := range writes {
		n += int64(len(w.Data))
	}
	d.counter.add(true, flush, n)
	return d.Device.Write(ctx, writes, flush)
}

// Read counts and forwards a read.
func (d countingDevice) Read(ctx context.Context, reads []device.Read) error {
	d.counter.add(false, false, 0)
	return d.Device.Read(ctx, reads)
}

// countingStore counts the calls made through a record store.
type countingStore struct {
	records.Store
	// counter holds the counts.
	counter *counter
}

// Get counts and forwards a get.
func (s countingStore) Get(ctx context.Context, keys [][]byte) ([][]byte, error) {
	s.counter.add(false, false, 0)
	return s.Store.Get(ctx, keys)
}

// Has counts and forwards a has.
func (s countingStore) Has(ctx context.Context, keys [][]byte) ([]bool, error) {
	s.counter.add(false, false, 0)
	return s.Store.Has(ctx, keys)
}

// Scan counts and forwards a scan.
func (s countingStore) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	s.counter.add(false, false, 0)
	return s.Store.Scan(ctx, prefix, fn)
}

// Commit counts and forwards a commit.
func (s countingStore) Commit(ctx context.Context, ops []records.Op, durable bool) error {
	var n int64
	for _, op := range ops {
		n += int64(len(op.Key) + len(op.Value))
	}
	s.counter.add(true, durable, n)
	return s.Store.Commit(ctx, ops, durable)
}

// countingBackend counts the calls made through a format 3 backend.
type countingBackend struct {
	opfs_engine.Backend
	counter *counter
}

// Read counts and forwards a read.
func (b countingBackend) Read(ctx context.Context, name string, offset int64, length int) ([]byte, error) {
	b.counter.add(false, false, 0)
	return b.Backend.Read(ctx, name, offset, length)
}

// Write counts and forwards a whole-file write.
func (b countingBackend) Write(ctx context.Context, name string, data []byte) error {
	b.counter.add(true, true, int64(len(data)))
	return b.Backend.Write(ctx, name, data)
}

// Remove counts and forwards a remove.
func (b countingBackend) Remove(ctx context.Context, name string) error {
	b.counter.add(true, false, 0)
	return b.Backend.Remove(ctx, name)
}

// openE1 opens E1 on a device from open, which reopens the same storage.
func openE1(ctx context.Context, open func() (device.Device, func() error, error), destroy func() error) (engine, error) {
	c := &counter{}
	start := func() (*paylog.Store, func() error, error) {
		dev, closeDevice, err := open()
		if err != nil {
			return nil, nil, err
		}
		counted := countingDevice{Device: dev, counter: c}
		idx, err := logindex.Open(ctx, counted, logindex.Options{})
		if err != nil {
			_ = closeDevice()
			return nil, nil, err
		}
		s, err := paylog.Open(ctx, counted, idx)
		if err != nil {
			_ = closeDevice()
			return nil, nil, err
		}
		return s, func() error {
			_ = s.Close()
			return closeDevice()
		}, nil
	}
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

// openE1OPFS opens E1 on a fresh OPFS device.
func openE1OPFS(ctx context.Context) (engine, error) {
	path := storageName + "/e1"
	if err := device_opfs.Delete(path); err != nil {
		return engine{}, err
	}
	return openE1(ctx, func() (device.Device, func() error, error) {
		d, err := device_opfs.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}, func() error {
		return device_opfs.Delete(storageName)
	})
}

// openE1IDB opens E1 on a fresh IndexedDB device.
func openE1IDB(ctx context.Context) (engine, error) {
	name := storageName + "-e1"
	if err := volume_idb.DeleteDatabase(name); err != nil {
		return engine{}, err
	}
	return openE1(ctx, func() (device.Device, func() error, error) {
		d, err := volume_idb.OpenDevice(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}, func() error {
		return volume_idb.DeleteDatabase(name)
	})
}

// openE5IDB opens E5 on a fresh IndexedDB record store.
func openE5IDB(ctx context.Context) (engine, error) {
	name := storageName + "-e5"
	if err := volume_idb.DeleteDatabase(name); err != nil {
		return engine{}, err
	}
	c := &counter{}
	start := func() (*direct.Store, *volume_idb.Store, error) {
		rs, err := volume_idb.OpenStore(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		s, err := direct.Open(ctx, countingStore{Store: rs, counter: c})
		if err != nil {
			_ = rs.Close()
			return nil, nil, err
		}
		return s, rs, nil
	}
	s, rs, err := start()
	if err != nil {
		return engine{}, err
	}
	closeStore := func() error {
		if s == nil {
			return nil
		}
		_ = s.Close()
		return rs.Close()
	}
	return engine{
		target: s,
		reopen: func(ctx context.Context) (workload.Target, error) {
			if err := closeStore(); err != nil {
				return nil, err
			}
			s, rs, err = start()
			if err != nil {
				return nil, err
			}
			return s, nil
		},
		counts: c.snapshot,
		destroy: func() error {
			_ = closeStore()
			return volume_idb.DeleteDatabase(name)
		},
	}, nil
}

// openE4OPFS opens format 3 (E4) on a fresh OPFS directory through its
// browser backend.
func openE4OPFS(ctx context.Context) (engine, error) {
	path := storageName + "/e4"
	if err := device_opfs.Delete(path); err != nil {
		return engine{}, err
	}
	c := &counter{}
	start := func() (opfs_engine.ReplayTarget, error) {
		root, err := opfs.GetRoot()
		if err != nil {
			return opfs_engine.ReplayTarget{}, err
		}
		dir, err := opfs.GetDirectoryPath(root, strings.Split(path, "/"), true)
		if err != nil {
			return opfs_engine.ReplayTarget{}, err
		}
		backend := opfs_engine.NewBrowserBackend(opfs.DefaultDriver, dir, path)
		e, err := opfs_engine.Open(ctx, countingBackend{Backend: backend, counter: c})
		if err != nil {
			_ = backend.Close()
			return opfs_engine.ReplayTarget{}, err
		}
		return opfs_engine.ReplayTarget{Engine: e, BlockStore: opfs_engine.NewBlockStore(ctx, e, 0)}, nil
	}
	t, err := start()
	if err != nil {
		return engine{}, err
	}
	closeTarget := func() error {
		if t.Engine == nil {
			return nil
		}
		_ = t.BlockStore.Close()
		return t.Engine.Close()
	}
	return engine{
		target: t,
		reopen: func(ctx context.Context) (workload.Target, error) {
			if err := closeTarget(); err != nil {
				return nil, err
			}
			t, err = start()
			if err != nil {
				return nil, err
			}
			return t, nil
		},
		counts: c.snapshot,
		destroy: func() error {
			_ = closeTarget()
			return device_opfs.Delete(storageName)
		},
	}, nil
}
