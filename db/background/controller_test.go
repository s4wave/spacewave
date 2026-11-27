package background

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/util/backoff"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// newTestBus starts a bus with a background job controller that retries
// after one millisecond.
func newTestBus(t *testing.T) bus.Bus {
	// Build the bus.
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	b, _, err := cbc.NewCoreBus(t.Context(), le)
	if err != nil {
		t.Fatal(err)
	}

	// Start the controller for the test's lifetime.
	conf := &Config{RetryBackoff: &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_CONSTANT,
		Constant:    &backoff.Constant{Interval: 1},
	}}
	release, err := b.AddController(t.Context(), NewController(le, conf), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return b
}

// waitJobState waits until the job with key reaches the state check accepts.
func waitJobState(t *testing.T, b bus.Bus, key string, run JobFunc, check func(*JobState) bool) *JobState {
	// Add a reference to the job and wait for the accepted state.
	t.Helper()
	state, _, ref, err := bus.ExecWaitValue(t.Context(), b, NewRunJob(key, run), nil, nil, func(state *JobState) (bool, error) {
		return check(state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref.Release()
	return state
}

// TestRunJobSharesKeyAndCancelsOnLastRelease verifies registrants of one key
// share one background run that stops when the last one releases it.
func TestRunJobSharesKeyAndCancelsOnLastRelease(t *testing.T) {
	// Build a job that runs as background work until it is canceled.
	ctx := t.Context()
	b := newTestBus(t)
	var runs atomic.Int32
	started := make(chan struct{})
	stopped := make(chan struct{})
	run := func(ctx context.Context) error {
		// Check the class and record the run.
		if !block.IsBackground(ctx) {
			t.Error("job context is not marked as background")
		}
		runs.Add(1)
		close(started)

		// Hold the run until the job is released.
		<-ctx.Done()
		close(stopped)
		return context.Canceled
	}

	// Register the same key twice.
	first, err := AddJob(ctx, b, "fill", run)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	second, err := AddJob(ctx, b, "fill", func(context.Context) error {
		t.Error("second registrant of the key started its own run")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Releasing one registrant keeps the shared run going.
	first.Release()
	select {
	case <-stopped:
		t.Fatal("job stopped while a registrant still held it")
	default:
	}

	// Releasing the last registrant cancels the shared run.
	second.Release()
	select {
	case <-t.Context().Done():
		t.Fatal("job was not canceled after its last release")
	case <-stopped:
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("job ran %d times, want 1", got)
	}
}

// TestRunJobRetriesAfterError verifies a failed attempt publishes its error and
// retries until an attempt succeeds.
func TestRunJobRetriesAfterError(t *testing.T) {
	// Fail the first attempt and hold the second until the retry state is seen.
	b := newTestBus(t)
	var attempts atomic.Int32
	retried := make(chan struct{})
	run := func(ctx context.Context) error {
		if attempts.Add(1) == 1 {
			return errors.New("link dropped")
		}
		<-retried
		return nil
	}
	ref, err := AddJob(t.Context(), b, "fill", run)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// The second attempt runs after the failure.
	running := waitJobState(t, b, "fill", run, func(state *JobState) bool {
		return state.GetRunning().GetAttempt() == 2
	})
	if running.GetRunning() == nil {
		t.Fatalf("unexpected state %v", running)
	}
	close(retried)

	// The job finishes after the successful attempt.
	waitJobState(t, b, "fill", run, func(state *JobState) bool {
		return state.GetDone() != nil
	})
	if got := attempts.Load(); got != 2 {
		t.Fatalf("job ran %d attempts, want 2", got)
	}
}

// TestRunJobOneAtATime verifies a second job waits for the run slot until the
// first finishes.
func TestRunJobOneAtATime(t *testing.T) {
	// Hold the only run slot with the first job.
	b := newTestBus(t)
	release := make(chan struct{})
	started := make(chan struct{})
	first, err := AddJob(t.Context(), b, "first", func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	<-started

	// The second job waits while the first runs.
	var secondRan atomic.Bool
	second := func(context.Context) error {
		secondRan.Store(true)
		return nil
	}
	secondRef, err := AddJob(t.Context(), b, "second", second)
	if err != nil {
		t.Fatal(err)
	}
	defer secondRef.Release()
	waitJobState(t, b, "second", second, func(state *JobState) bool {
		return state.GetWaiting() != nil
	})
	if secondRan.Load() {
		t.Fatal("second job ran while the first held the run slot")
	}

	// Finishing the first job lets the second run.
	close(release)
	waitJobState(t, b, "second", second, func(state *JobState) bool {
		return state.GetDone() != nil
	})
	if !secondRan.Load() {
		t.Fatal("second job finished without running")
	}
}

// TestExRunJobLoadsControllerAndWaitsForDone verifies ExRunJob starts the
// background controller from its factory and returns once a retried job is
// done.
func TestExRunJobLoadsControllerAndWaitsForDone(t *testing.T) {
	// Build a bus that can construct the controller but has not started it.
	le := logrus.NewEntry(logrus.New())
	b, sr, err := cbc.NewCoreBus(t.Context(), le)
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(NewFactory(b))

	// Run a job whose first attempt fails.
	var attempts atomic.Int32
	err = ExRunJob(t.Context(), b, "flaky", func(ctx context.Context) error {
		if attempts.Add(1) == 1 {
			return errors.New("first attempt fails")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("job ran %d attempts, want 2", got)
	}
}
