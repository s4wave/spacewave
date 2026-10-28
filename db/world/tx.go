package world

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// Tx implements the world state transaction interfaces.
//
// Concurrent calls to WorldState functions should be supported.
type Tx interface {
	// WorldState contains the world read/write ops.
	WorldState
	// Tx contains the transaction Commit/Discard ops.
	tx.Tx
}

// orderedCommitKey marks a context whose write transactions may commit ordered.
type orderedCommitKey struct{}

// WithOrderedCommit marks write transactions committed with ctx as allowed to
// commit ordered: Commit returns once the change is applied and safe against a
// process crash, and the change becomes durable at the engine's next Sync or
// its storage's own durability point. Engines without ordered commits ignore
// the mark and commit in full.
func WithOrderedCommit(ctx context.Context) context.Context {
	return context.WithValue(ctx, orderedCommitKey{}, true)
}

// OrderedCommit reports whether ctx carries WithOrderedCommit.
func OrderedCommit(ctx context.Context) bool {
	ordered, _ := ctx.Value(orderedCommitKey{}).(bool)
	return ordered
}

// optimisticWriteKey marks a context whose write transactions run optimistically.
type optimisticWriteKey struct{}

// WithOptimisticWrite asks the engine to run the write transactions opened with
// ctx optimistically: the write forks from the current World without keeping
// the writer lock while the caller uses it, and its Commit fails with an error
// wrapping coord.ErrStaleGeneration when a write committed since the fork
// changed what it read. Run it again then, without the request, as
// ExecTransaction does. Engines that always hold the writer ignore the request.
func WithOptimisticWrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, optimisticWriteKey{}, true)
}

// WithoutOptimisticWrite removes the request WithOptimisticWrite added, so a
// rerun holds the writer lock and cannot conflict again.
func WithoutOptimisticWrite(ctx context.Context) context.Context {
	if !OptimisticWrite(ctx) {
		return ctx
	}
	return context.WithValue(ctx, optimisticWriteKey{}, false)
}

// OptimisticWrite reports whether ctx carries WithOptimisticWrite.
func OptimisticWrite(ctx context.Context) bool {
	optimistic, _ := ctx.Value(optimisticWriteKey{}).(bool)
	return optimistic
}
