package packcopy

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// ErrBudget stops a copy whose source has served its byte budget.
var ErrBudget = errors.New("read budget spent")

// Meter is an object store that counts the bytes read through it and refuses
// reads once they reach a budget, for a source that charges or caps transfer.
type Meter struct {
	block_store_s3.ObjectStore
	// budget is the bytes allowed, zero for no limit.
	budget uint64
	// used is the bytes read so far, across runs.
	used atomic.Uint64
}

// NewMeter wraps objects with a budget, starting from the bytes earlier runs
// used. A zero budget counts without limit.
func NewMeter(objects block_store_s3.ObjectStore, budget, used uint64) *Meter {
	m := &Meter{ObjectStore: objects, budget: budget}
	m.used.Store(used)
	return m
}

// Used returns the bytes read so far.
func (m *Meter) Used() uint64 {
	return m.used.Load()
}

// check refuses a read past the budget.
func (m *Meter) check() error {
	if m.budget != 0 && m.used.Load() >= m.budget {
		return ErrBudget
	}
	return nil
}

// GetObject opens the object, counting what the caller reads.
func (m *Meter) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	body, err := m.ObjectStore.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	return &meteredBody{ReadCloser: body, used: &m.used}, nil
}

// GetObjectRange reads the range, counting it.
func (m *Meter) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	data, err := m.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
	m.used.Add(uint64(len(data)))
	return data, err
}

// meteredBody counts the bytes read from an object's body.
type meteredBody struct {
	io.ReadCloser
	// used receives the count.
	used *atomic.Uint64
}

// Read reads and counts.
func (b *meteredBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.used.Add(uint64(n)) //nolint:gosec // a read count is never negative.
	return n, err
}

// _ is a type assertion.
var _ block_store_s3.ObjectStore = (*Meter)(nil)
