package kvtx

import (
	"context"
	"errors"
	"os"
)

// optimisticAttempts bounds the attempts RunOptimisticTransaction opens
// optimistically before it opens one that holds the writer lock throughout.
const optimisticAttempts = 2

// OptimisticEnv names the environment variable that turns optimistic write
// transactions off: while it is "off", RunOptimisticTransaction opens every
// attempt holding the writer lock throughout, as RunTransactionWithRetry does.
const OptimisticEnv = "SPACEWAVE_OPTIMISTIC_TX"

// optimisticContextKey marks the context a write transaction is opened with
// as optimistic.
type optimisticContextKey struct{}

// IsOptimisticContext reports whether a write transaction opened with ctx
// may run optimistically: read a snapshot without the store's writer lock,
// take the lock only at Commit, and fail Commit with ErrConflict when a
// commit after the snapshot changed what it read. Only
// RunOptimisticTransaction marks a context, and only for opening, so a
// transaction the body opens itself is never optimistic. Stores without an
// optimistic mode ignore the mark.
func IsOptimisticContext(ctx context.Context) bool {
	optimistic, _ := ctx.Value(optimisticContextKey{}).(bool)
	return optimistic
}

// RunOptimisticTransaction runs body in a write transaction that a store
// with an optimistic mode opens without its writer lock, so the body's reads,
// cold or not, delay no other writer. Commit takes the lock and validates the
// reads; on ErrConflict the body runs again on a fresh transaction, and after
// optimisticAttempts conflicts it runs holding the lock throughout, so it
// always finishes. Other failures retry as RunTransactionWithRetry does with
// retry.
//
// The body must be safe to replay, as for RunTransaction, and also safe to
// run concurrently with other bodies, since the lock no longer serializes
// them: shared memory it touches needs its own lock.
func RunOptimisticTransaction[T TransactionLifecycle](
	ctx context.Context,
	open func(context.Context) (T, error),
	body func(context.Context, T) error,
	retry RetryPredicate,
) error {
	// Mark the opening context of the first attempts, and count conflicts.
	conflicts := 0
	attempt := func(ctx context.Context) error {
		// Open optimistically until the conflicts run out or the mode is off.
		openCtx := ctx
		if conflicts < optimisticAttempts && os.Getenv(OptimisticEnv) != "off" {
			openCtx = context.WithValue(ctx, optimisticContextKey{}, true)
		}

		// Run the attempt, counting a conflict.
		err := runTransactionAttempt(ctx, openCtx, true, open, body)
		if errors.Is(err, ErrConflict) {
			conflicts++
		}
		return err
	}

	// Retry conflicts and what retry allows.
	return runOperationWithRetry(
		ctx,
		attempt,
		func(err error) bool { return errors.Is(err, ErrConflict) || retry(err) },
		"kvtx optimistic transaction attempt limit exhausted",
	)
}
