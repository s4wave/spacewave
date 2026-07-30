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

// RunTransaction executes body against a fresh transaction for each retryable
// failure and retries only ErrInvalidSnapshot. A successful read body is
// discarded without committing; a successful write body is committed before
// the attempt is discarded.
//
// The body must be safe to replay. It may use the transaction and perform
// deterministic local computation, but external effects and mutable
// accumulators that span attempts belong outside the body.
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
	for range transactionAttemptLimit {
		// Cancellation between attempts must win before another transaction opens.
		if err := ctx.Err(); err != nil {
			return err
		}

		err := runTransactionAttempt(ctx, write, open, body)
		if err == nil {
			return nil
		}
		if !retry(err) {
			return err
		}
	}

	return errors.New("kvtx transaction attempt limit exhausted")
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
	if err := body(ctx, tx); err != nil {
		return err
	}
	// Body cancellation must prevent a write from committing partial work.
	if err := ctx.Err(); err != nil {
		return err
	}
	if !write {
		return nil
	}
	return tx.Commit(ctx)
}
