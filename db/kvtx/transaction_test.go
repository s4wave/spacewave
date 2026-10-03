package kvtx

import (
	"context"
	"errors"
	"testing"
)

type retryTestTx struct {
	id        int
	events    *[]string
	commitErr error
}

func (t *retryTestTx) Commit(context.Context) error {
	*t.events = append(*t.events, "commit:"+string(rune('0'+t.id)))
	return t.commitErr
}

func (t *retryTestTx) Discard() {
	*t.events = append(*t.events, "discard:"+string(rune('0'+t.id)))
}

func TestRunTransactionRetriesFreshAttemptsAndDiscards(t *testing.T) {
	// Record transaction opens and lifecycle events across fresh retry attempts.
	var events []string
	var opened int

	// Run a write transaction whose first body attempt requests a retry.
	err := RunTransaction(
		context.Background(),
		true,
		func(context.Context) (*retryTestTx, error) {
			opened++
			return &retryTestTx{id: opened, events: &events}, nil
		},
		func(_ context.Context, tx *retryTestTx) error {
			events = append(events, "body:"+string(rune('0'+tx.id)))
			if tx.id == 1 {
				return errors.Join(errors.New("backend detail"), ErrInvalidSnapshot)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Compare the retry, commit, and discard event sequence.
	want := []string{
		"body:1",
		"discard:1",
		"body:2",
		"commit:2",
		"discard:2",
	}

	// Require exactly one successful commit after the retry.
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
	if opened != 2 {
		t.Fatalf("opened = %d, want 2", opened)
	}
}

func TestRunTransactionRetriesTypedCommitFailure(t *testing.T) {
	// Record transaction lifecycle events for a typed commit failure.
	var events []string
	var opened int

	// Retry the write transaction after its first commit invalidates the snapshot.
	err := RunTransaction(
		context.Background(),
		true,
		func(context.Context) (*retryTestTx, error) {
			opened++
			var commitErr error
			if opened == 1 {
				commitErr = ErrInvalidSnapshot
			}
			return &retryTestTx{id: opened, events: &events, commitErr: commitErr}, nil
		},
		func(_ context.Context, tx *retryTestTx) error {
			events = append(events, "body:"+string(rune('0'+tx.id)))
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the failed commit is discarded before a fresh attempt commits.
	want := []string{
		"body:1",
		"commit:1",
		"discard:1",
		"body:2",
		"commit:2",
		"discard:2",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestRunTransactionDiscardsBodyErrorWithoutCommit(t *testing.T) {
	// Prepare a transaction body failure and its lifecycle event log.
	var events []string
	bodyErr := errors.New("body failed")

	// Run a write transaction whose body fails without permitting a retry.
	err := RunTransaction(
		context.Background(),
		true,
		func(context.Context) (*retryTestTx, error) {
			return &retryTestTx{id: 1, events: &events}, nil
		},
		func(_ context.Context, tx *retryTestTx) error {
			events = append(events, "body:"+string(rune('0'+tx.id)))
			return bodyErr
		},
	)
	if !errors.Is(err, bodyErr) {
		t.Fatalf("error = %v, want body error", err)
	}

	// Verify the failed transaction body is discarded without committing.
	want := []string{"body:1", "discard:1"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestRunOperationReturnsOperationExhaustionError(t *testing.T) {
	attempts := 0
	err := RunOperationWithRetry(
		context.Background(),
		func(context.Context) error {
			attempts++
			return ErrInvalidSnapshot
		},
		RetryInvalidSnapshot,
	)
	if err == nil || err.Error() != "kvtx operation attempt limit exhausted" {
		t.Fatalf("error = %v, want operation attempt limit exhaustion", err)
	}
	if attempts != transactionAttemptLimit {
		t.Fatalf("attempts = %d, want %d", attempts, transactionAttemptLimit)
	}
}

func TestRunTransactionReturnsExhaustionErrorAtAttemptBound(t *testing.T) {
	// Record transaction attempts and prepare a repeatedly failing body.
	var events []string
	var opened int
	bodyErr := errors.New("body failed")

	// Run read transactions until the configured attempt limit is reached.
	err := RunTransactionWithRetry(
		context.Background(),
		false,
		func(context.Context) (*retryTestTx, error) {
			opened++
			return &retryTestTx{id: opened, events: &events}, nil
		},
		func(context.Context, *retryTestTx) error {
			return bodyErr
		},
		func(error) bool {
			return true
		},
	)
	if err == nil || err.Error() != "kvtx transaction attempt limit exhausted" {
		t.Fatalf("error = %v, want transaction attempt limit exhaustion", err)
	}
	if errors.Is(err, bodyErr) {
		t.Fatalf("error = %v, want exhaustion error instead of last body error", err)
	}
	if opened != transactionAttemptLimit {
		t.Fatalf("opened = %d, want %d", opened, transactionAttemptLimit)
	}
	if len(events) != transactionAttemptLimit {
		t.Fatalf("discard count = %d, want %d", len(events), transactionAttemptLimit)
	}
}

func TestRunTransactionCancellationBeforeOpen(t *testing.T) {
	// Cancel the transaction context before any attempt can open.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run a write transaction with the already canceled context.
	opened := 0
	err := RunTransaction(
		ctx,
		true,
		func(context.Context) (*retryTestTx, error) {
			opened++
			return &retryTestTx{events: new([]string)}, nil
		},
		func(context.Context, *retryTestTx) error {
			t.Fatal("body called after cancellation")
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if opened != 0 {
		t.Fatalf("opened = %d, want 0", opened)
	}
}

func TestRunTransactionCancellationDuringOpenSkipsBody(t *testing.T) {
	// Prepare a cancelable context for transaction opening.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel during transaction opening and record whether the body runs.
	var events []string
	bodyCalled := false
	err := RunTransaction(
		ctx,
		true,
		func(context.Context) (*retryTestTx, error) {
			cancel()
			return &retryTestTx{id: 1, events: &events}, nil
		},
		func(context.Context, *retryTestTx) error {
			bodyCalled = true
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if bodyCalled {
		t.Fatal("body called after open canceled the context")
	}
	if len(events) != 1 || events[0] != "discard:1" {
		t.Fatalf("events = %v, want one discard", events)
	}
}

func TestRunTransactionCancellationDuringBodySkipsCommit(t *testing.T) {
	// Prepare a cancelable context for the transaction body.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel the transaction body and record its cleanup sequence.
	var events []string
	var opened int
	err := RunTransaction(
		ctx,
		true,
		func(context.Context) (*retryTestTx, error) {
			opened++
			return &retryTestTx{id: opened, events: &events}, nil
		},
		func(context.Context, *retryTestTx) error {
			events = append(events, "body:1")
			cancel()
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if opened != 1 {
		t.Fatalf("opened = %d, want 1", opened)
	}
	if len(events) != 2 || events[0] != "body:1" || events[1] != "discard:1" {
		t.Fatalf("events = %v, want body then discard", events)
	}
}

func TestRunTransactionDoesNotRetryDiagnosticText(t *testing.T) {
	// Prepare a diagnostic error without the invalid-snapshot marker.
	var opened int
	textErr := errors.New("panic: page 2 already freed")

	// Run a transaction whose body returns the diagnostic error.
	err := RunTransaction(
		context.Background(),
		false,
		func(context.Context) (*retryTestTx, error) {
			opened++
			return &retryTestTx{id: opened, events: new([]string)}, nil
		},
		func(context.Context, *retryTestTx) error {
			return textErr
		},
	)
	if !errors.Is(err, textErr) {
		t.Fatalf("error = %v, want diagnostic error", err)
	}
	if opened != 1 {
		t.Fatalf("opened = %d, want 1", opened)
	}
}

func TestRunTransactionRetriesTypedOpenFailure(t *testing.T) {
	// Record transaction opens and cleanup after a typed opening failure.
	var events []string
	var opened int

	// Retry transaction opening when the first snapshot is invalid.
	err := RunTransaction(
		context.Background(),
		false,
		func(context.Context) (*retryTestTx, error) {
			opened++
			if opened == 1 {
				return nil, errors.Join(errors.New("stale generation"), ErrInvalidSnapshot)
			}
			return &retryTestTx{id: opened, events: &events}, nil
		},
		func(context.Context, *retryTestTx) error {
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if opened != 2 {
		t.Fatalf("opened = %d, want 2", opened)
	}
	if len(events) != 1 || events[0] != "discard:2" {
		t.Fatalf("events = %v, want only successful attempt discard", events)
	}
}
