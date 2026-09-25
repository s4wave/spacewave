//go:build goscript

package goscript_volume_replay

import (
	"context"
	"syscall/js"

	"github.com/s4wave/spacewave/db/volume/direct"
	"github.com/s4wave/spacewave/db/volume/records"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// sqliteWorkerPath is the fixture's SQLite device worker script. The query
// disables the SQLite VFSes that need an async proxy worker, leaving
// opfs-sahpool.
const sqliteWorkerPath = "/workers/sqlite-records.js?opfs-disable&opfs-wl-disable"

// sqliteStore is a records.Store in a SQLite database owned by a dedicated
// device worker, reached by one message per call.
type sqliteStore struct {
	*workerClient
}

// open opens the database name in the worker.
func (s *sqliteStore) open(ctx context.Context, name string) error {
	_, err := s.call(ctx, "open", func(req js.Value) { req.Set("name", name) })
	return err
}

// Get reads the records of keys.
func (s *sqliteStore) Get(ctx context.Context, keys [][]byte) ([][]byte, error) {
	resp, err := s.call(ctx, "get", func(req js.Value) { req.Set("keys", bytesArray(keys)) })
	if err != nil {
		return nil, err
	}
	values := resp.Get("values")
	out := make([][]byte, len(keys))
	for i := range out {
		if v := values.Index(i); !v.IsUndefined() {
			out[i] = toGo(v)
		}
	}
	return out, nil
}

// Has reports which keys have records.
func (s *sqliteStore) Has(ctx context.Context, keys [][]byte) ([]bool, error) {
	resp, err := s.call(ctx, "has", func(req js.Value) { req.Set("keys", bytesArray(keys)) })
	if err != nil {
		return nil, err
	}
	has := resp.Get("has")
	out := make([]bool, len(keys))
	for i := range out {
		out[i] = has.Index(i).Bool()
	}
	return out, nil
}

// Scan reads every record under prefix, then calls fn with each in key order.
func (s *sqliteStore) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	resp, err := s.call(ctx, "scan", func(req js.Value) { req.Set("prefix", toJS(prefix)) })
	if err != nil {
		return err
	}
	keys, values := resp.Get("keys"), resp.Get("values")
	for i := range keys.Length() {
		if err := fn(toGo(keys.Index(i)), toGo(values.Index(i))); err != nil {
			return err
		}
	}
	return nil
}

// Commit applies ops in one transaction, synced when durable is set.
func (s *sqliteStore) Commit(ctx context.Context, ops []records.Op, durable bool) error {
	_, err := s.call(ctx, "commit", func(req js.Value) {
		arr := js.Global().Get("Array").New(len(ops))
		for i, op := range ops {
			o := js.Global().Get("Object").New()
			o.Set("key", toJS(op.Key))
			if !op.Delete {
				o.Set("value", toJS(op.Value))
			}
			arr.SetIndex(i, o)
		}
		req.Set("ops", arr)
		req.Set("durable", durable)
	})
	return err
}

// close closes the database, keeping the worker for a reopen.
func (s *sqliteStore) close(ctx context.Context) error {
	_, err := s.call(ctx, "close", nil)
	return err
}

// destroy deletes every database file and stops the worker.
func (s *sqliteStore) destroy(ctx context.Context) error {
	_, err := s.call(ctx, "destroy", nil)
	s.terminate()
	return err
}

// bytesArray copies keys into an array of Uint8Arrays.
func bytesArray(keys [][]byte) js.Value {
	arr := js.Global().Get("Array").New(len(keys))
	for i, key := range keys {
		arr.SetIndex(i, toJS(key))
	}
	return arr
}

// openE3SQLite opens E3 on a fresh SQLite database in a device worker (T2).
func openE3SQLite(ctx context.Context) (engine, error) {
	rs := &sqliteStore{workerClient: startWorker(sqliteWorkerPath)}
	if _, err := rs.call(ctx, "destroy", nil); err != nil {
		_ = rs.destroy(ctx)
		return engine{}, err
	}
	c := &counter{}
	start := func() (*direct.Store, error) {
		if err := rs.open(ctx, storageName+"-e3"); err != nil {
			return nil, err
		}
		s, err := direct.Open(ctx, countingStore{Store: rs, counter: c})
		if err != nil {
			_ = rs.close(ctx)
			return nil, err
		}
		return s, nil
	}
	s, err := start()
	if err != nil {
		_ = rs.destroy(ctx)
		return engine{}, err
	}
	return engine{
		target: s,
		reopen: func(ctx context.Context) (workload.Target, error) {
			_ = s.Close()
			if err := rs.close(ctx); err != nil {
				return nil, err
			}
			s, err = start()
			if err != nil {
				return nil, err
			}
			return s, nil
		},
		counts: c.snapshot,
		destroy: func() error {
			if s != nil {
				_ = s.Close()
			}
			return rs.destroy(context.Background())
		},
	}, nil
}

// _ checks that sqliteStore is a records.Store.
var _ records.Store = (*sqliteStore)(nil)
