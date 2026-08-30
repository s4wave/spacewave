package kvtx

import (
	"context"
	"errors"
)

// transactionAttemptLimit bounds the number of complete transaction attempts
// for one logical operation, including attempts that fail while opening.
const transactionAttemptLimit = 10

// TransactionLifecycle is the lifecycle a retryable transaction must expose.
type TransactionLifecycle interface {
	Commit(ctx context.Context) error
	Discard()
}

// RetryPredicate reports whether an error permits a fresh transaction attempt.
// Predicates must classify typed errors with errors.Is; diagnostic text is not
// part of the transaction retry contract.
type RetryPredicate func(error) bool

// RetryInvalidSnapshot reports whether err requires a fresh storage snapshot.
func RetryInvalidSnapshot(err error) bool {
	return errors.Is(err, ErrInvalidSnapshot)
}

// RunOperation executes a complete logical operation again when its storage
// snapshot becomes invalid. The body must be safe to replay.
func RunOperation(ctx context.Context, body func(context.Context) error) error {
	return RunOperationWithRetry(ctx, body, RetryInvalidSnapshot)
}

// RunOperationWithRetry executes a complete logical operation with the
// supplied typed retry policy. The body must not perform non-idempotent external
// effects. Per-attempt mutable state must be created or reset inside the body;
// immutable inputs may be captured from outside it.
func RunOperationWithRetry(
	ctx context.Context,
	body func(context.Context) error,
	retry RetryPredicate,
) error {
	return runOperationWithRetry(
		ctx,
		body,
		retry,
		"kvtx operation attempt limit exhausted",
	)
}

func runOperationWithRetry(
	ctx context.Context,
	body func(context.Context) error,
	retry RetryPredicate,
	exhaustedError string,
) error {
	for range transactionAttemptLimit {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := body(ctx)
		if err == nil {
			return nil
		}
		if !retry(err) {
			return err
		}
	}

	return errors.New(exhaustedError)
}

// RunTransaction executes body against a fresh transaction for each retryable
// failure and retries only ErrInvalidSnapshot. A successful read body is
// discarded without committing; a successful write body is committed before
// the attempt is discarded.
//
// The body must be safe to replay. It may use the transaction and perform
// deterministic local computation, but it must not perform non-idempotent
// external effects. Per-attempt mutable state must be reset inside the body.
func RunTransaction[T TransactionLifecycle](
	ctx context.Context,
	write bool,
	open func(context.Context) (T, error),
	body func(context.Context, T) error,
) error {
	return RunTransactionWithRetry(ctx, write, open, body, RetryInvalidSnapshot)
}

// RunTransactionWithRetry executes body with the supplied typed retry policy.
// Every opened attempt is discarded, including attempts whose body or commit
// fails. The next attempt opens only after the failed attempt has returned.
func RunTransactionWithRetry[T TransactionLifecycle](
	ctx context.Context,
	write bool,
	open func(context.Context) (T, error),
	body func(context.Context, T) error,
	retry RetryPredicate,
) error {
	return runOperationWithRetry(
		ctx,
		func(ctx context.Context) error {
			return runTransactionAttempt(ctx, write, open, body)
		},
		retry,
		"kvtx transaction attempt limit exhausted",
	)
}

func runTransactionAttempt[T TransactionLifecycle](
	ctx context.Context,
	write bool,
	open func(context.Context) (T, error),
	body func(context.Context, T) error,
) error {
	tx, err := open(ctx)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Opening may cancel the context while still returning a live transaction.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Execute the replayable transaction body.
	if err := body(ctx, tx); err != nil {
		return err
	}

	// Body cancellation must prevent a write from committing partial work.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Commit completed write work before the deferred discard.
	if !write {
		return nil
	}
	return tx.Commit(ctx)
}
