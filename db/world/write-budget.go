package world

import (
	"context"
	"sync/atomic"
	"time"
)

// WriteBudgetLength is how long one World write may hold the engine's writer
// while it waits on plugin operation handlers.
const WriteBudgetLength = 10 * time.Second

// WriteBudget bounds the time one World write may spend waiting on plugin
// operation handlers while it holds the engine's writer. It starts once, when
// the write first acquires the writer, and is never renewed: a replay of the
// write after another writer moved the head spends the same budget, and the
// time a replay waits to acquire the writer again counts against it.
//
// The budget bounds plugin time under the writer. The budget is not a deadline
// for the write. Only plugin calls are cut at the deadline. An operation
// implemented in Go runs to completion, and the budget ends the write's replay
// loop only after a plugin call claimed it with Deadline, so a write of Go
// operations alone replays as long as it did without a budget.
//
// A WriteBudget is safe for concurrent use: nested operations arrive over
// plugin RPC calls on other goroutines.
type WriteBudget struct {
	// length is the budget's length.
	length time.Duration
	// start is the Unix nanosecond time Start was first called, or zero.
	start atomic.Int64
	// claimed is set once a plugin call took the deadline.
	claimed atomic.Bool
}

// NewWriteBudget constructs a WriteBudget of the given length, not started.
func NewWriteBudget(length time.Duration) *WriteBudget {
	return &WriteBudget{length: length}
}

// GetLength returns the budget's length.
func (b *WriteBudget) GetLength() time.Duration {
	return b.length
}

// Start starts the budget at the current time. Only the first call counts.
func (b *WriteBudget) Start() {
	b.start.CompareAndSwap(0, time.Now().UnixNano())
}

// Deadline returns the time the budget ends and marks it claimed by a plugin
// call. It returns false before Start.
func (b *WriteBudget) Deadline() (time.Time, bool) {
	start := b.start.Load()
	if start == 0 {
		return time.Time{}, false
	}
	b.claimed.Store(true)
	return time.Unix(0, start).Add(b.length), true
}

// Spent reports whether a plugin call claimed the budget and its deadline has
// passed. A nil budget is never spent.
func (b *WriteBudget) Spent() bool {
	if b == nil || !b.claimed.Load() {
		return false
	}
	start := b.start.Load()
	return start != 0 && !time.Now().Before(time.Unix(0, start).Add(b.length))
}

// writeBudgetKey is the context key of the WriteBudget.
type writeBudgetKey struct{}

// WithWriteBudget returns ctx carrying b. A ctx that already carries a budget
// keeps it, so the outermost write's budget governs every nested write. A nil
// b returns ctx.
func WithWriteBudget(ctx context.Context, b *WriteBudget) context.Context {
	if b == nil || GetWriteBudget(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, writeBudgetKey{}, b)
}

// GetWriteBudget returns the WriteBudget ctx carries, or nil.
func GetWriteBudget(ctx context.Context) *WriteBudget {
	b, _ := ctx.Value(writeBudgetKey{}).(*WriteBudget)
	return b
}
