package writequeue

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/viewstream"
)

// Options binds queue scheduling to transaction coordination and observations.
// Callbacks must be concurrency-safe and outlive the queue.
type Options struct {
	// Wrap joins auxiliary transaction state to each World write. Nil uses World alone.
	Wrap func(context.Context, world.Tx) Transaction
	// BodyBudget bounds how long bodies run before the next batch. Zero uses 250ms.
	BodyBudget time.Duration
	// Owners receives durable actor revisions, or nil to disable actor notifications.
	Owners *viewstream.Writes
	// Oversized identifies an engine's commit size refusal so batches can split.
	Oversized func(error) bool
	// Unchanged identifies a body that failed without mutating its transaction.
	Unchanged error
	// Name returns the operation name used in diagnostics.
	Name func(context.Context) string
	// Applied returns the operation count used in slow-body diagnostics.
	Applied func(context.Context) int64
	// Subjects returns additional actors whose access this write changes.
	Subjects func(context.Context) []uint64
	// Committed counts a durably installed batch.
	Committed func()
}

// name returns the caller's diagnostic name, when supplied.
func (q *Queue) name(ctx context.Context) string {
	if q.options.Name == nil {
		return ""
	}
	return q.options.Name(ctx)
}

// applied reads the caller's operation count, when supplied.
func (q *Queue) applied(ctx context.Context) int64 {
	if q.options.Applied == nil {
		return 0
	}
	return q.options.Applied(ctx)
}

// subjects reads the additional actors this write changes, when supplied.
func (q *Queue) subjects(ctx context.Context) []uint64 {
	if q.options.Subjects == nil {
		return nil
	}
	return q.options.Subjects(ctx)
}

// oversized recognizes the engine's commit size refusal.
func (q *Queue) oversized(err error) bool {
	return q.options.Oversized != nil && q.options.Oversized(err)
}
