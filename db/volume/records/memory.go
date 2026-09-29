package records

import (
	"bytes"
	"context"
	"math/rand/v2"
	"sync"

	"github.com/pkg/errors"
	"github.com/tidwall/btree"
)

// ErrCrashed reports a call on a memory store after its injected crash.
var ErrCrashed = errors.New("record store crashed")

// Stats counts a store's transactions.
type Stats struct {
	// Reads counts Get, Has, and Scan calls.
	Reads int
	// Commits counts Commit calls.
	Commits int
	// Durable counts durable Commit calls.
	Durable int
	// Bytes counts the key and value bytes committed.
	Bytes int64
}

// Memory is an in-memory Store that simulates power loss. It keeps the durable
// records as of the last durable commit and the commits made since, so
// PowerLoss can keep any prefix of them.
type Memory struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// records is the current image every read sees.
	records *btree.Map[string, []byte]
	// durable is the image as of the last durable commit.
	durable *btree.Map[string, []byte]
	// pending holds the commits since the last durable commit, in order.
	pending [][]Op
	// crashAfter is the number of further commits that succeed before the
	// crash; negative never crashes.
	crashAfter int
	// crashed rejects every call until PowerLoss.
	crashed bool
	// stats counts the calls.
	stats Stats
}

// NewMemory returns an empty memory store that never crashes.
func NewMemory() *Memory {
	return &Memory{
		records:    new(btree.Map[string, []byte]),
		durable:    new(btree.Map[string, []byte]),
		crashAfter: -1,
	}
}

// CrashAfter crashes the store after n further commits succeed. The crashing
// commit may or may not apply, and every later call fails with ErrCrashed
// until PowerLoss. A negative n never crashes.
func (m *Memory) CrashAfter(n int) {
	m.mtx.Lock()
	m.crashAfter = n
	m.mtx.Unlock()
}

// Calls returns the number of Commit calls made.
func (m *Memory) Calls() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.stats.Commits
}

// Stats returns the counts so far.
func (m *Memory) Stats() Stats {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.stats
}

// PowerLoss replaces the records with an image a power loss could leave: the
// durable records and a prefix of the later commits, as rng chooses. The store
// then accepts calls again.
func (m *Memory) PowerLoss(rng *rand.Rand) {
	// Rebuild the records from the durable image plus a prefix of pending commits.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	image := m.durable.Copy()
	for _, ops := range m.pending[:rng.IntN(len(m.pending)+1)] {
		apply(image, ops)
	}
	m.records, m.durable, m.pending = image, image.Copy(), nil

	// Accept calls again.
	m.crashed, m.crashAfter = false, -1
}

// Get reads the records of keys, nil for an absent key.
func (m *Memory) Get(ctx context.Context, keys [][]byte) ([][]byte, error) {
	// Count the read and return a copy of each key's record.
	if err := m.read(ctx); err != nil {
		return nil, err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	values := make([][]byte, len(keys))
	for i, key := range keys {
		if v, ok := m.records.Get(string(key)); ok {
			values[i] = bytes.Clone(v)
		}
	}
	return values, nil
}

// Has reports which keys have records.
func (m *Memory) Has(ctx context.Context, keys [][]byte) ([]bool, error) {
	// Count the read and report each key's presence.
	if err := m.read(ctx); err != nil {
		return nil, err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	has := make([]bool, len(keys))
	for i, key := range keys {
		_, has[i] = m.records.Get(string(key))
	}
	return has, nil
}

// Scan calls fn with each record whose key has prefix, in key order, over the
// records as of the call.
func (m *Memory) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	// Count the read and copy the records for the scan.
	if err := m.read(ctx); err != nil {
		return err
	}
	m.mtx.Lock()
	records := m.records.Copy()
	m.mtx.Unlock()

	// Call fn with each record whose key has the prefix.
	var err error
	records.Ascend(string(prefix), func(key string, value []byte) bool {
		if !bytes.HasPrefix([]byte(key), prefix) {
			return false
		}
		err = fn([]byte(key), bytes.Clone(value))
		return err == nil
	})
	return err
}

// read counts a read call and checks that the store has not crashed.
func (m *Memory) read(ctx context.Context) error {
	// Reject a cancelled caller before counting the read.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Count the read and reject calls after a crash.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.stats.Reads++
	if m.crashed {
		return ErrCrashed
	}
	return nil
}

// Commit applies ops atomically, injecting the crash when it is due. The
// crashing commit joins the pending ones, so PowerLoss may keep it.
func (m *Memory) Commit(ctx context.Context, ops []Op, durable bool) error {
	// Reject a cancelled caller and count the commit.
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.stats.Commits++
	if m.crashed {
		return ErrCrashed
	}

	// Retain a copy of the ops as pending and charge their bytes.
	ops = cloneOps(ops)
	for _, op := range ops {
		m.stats.Bytes += int64(len(op.Key) + len(op.Value))
	}
	m.pending = append(m.pending, ops)

	// Inject the crash when it is due.
	if m.crashAfter == 0 {
		m.crashed = true
		return ErrCrashed
	}
	if m.crashAfter > 0 {
		m.crashAfter--
	}

	// Apply the ops and make them durable when requested.
	apply(m.records, ops)
	if durable {
		m.stats.Durable++
		for _, ops := range m.pending {
			apply(m.durable, ops)
		}
		m.pending = nil
	}
	return nil
}

// apply applies ops to image.
func apply(image *btree.Map[string, []byte], ops []Op) {
	for _, op := range ops {
		if op.Delete {
			image.Delete(string(op.Key))
			continue
		}
		image.Set(string(op.Key), op.Value)
	}
}

// cloneOps returns a deep copy of ops.
func cloneOps(ops []Op) []Op {
	out := make([]Op, len(ops))
	for i, op := range ops {
		out[i] = Op{Key: bytes.Clone(op.Key), Value: bytes.Clone(op.Value), Delete: op.Delete}
	}
	return out
}

// _ is a type assertion
var _ Store = (*Memory)(nil)
