package provider_gccleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestRunnerCoalescesTriggerDuringSweep(t *testing.T) {
	// Construct a Runner whose first sweep can be held open for another trigger.
	runner, _ := newTestRunner(t)

	// Gate collection and signal when each sweep begins.
	var calls int
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	runner.collect = func(ctx context.Context) (*block_gc.Stats, error) {
		calls++
		switch calls {
		case 1:
			close(firstStarted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-firstRelease:
			}
		case 2:
			close(secondStarted)
		}
		return &block_gc.Stats{}, nil
	}

	// Start the Runner so the test can trigger collection while it is active.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	// Begin the first sweep and wait until the collector blocks.
	runner.Trigger()
	waitSignal(t, firstStarted, "first sweep")

	// Queue another generation and release the blocked collection.
	runner.Trigger()
	close(firstRelease)
	waitSignal(t, secondStarted, "second sweep")

	// Wait for both generations and verify their completion.
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	if got := runner.CompletedGeneration(); got != 2 {
		t.Fatalf("completed generation = %d, want 2", got)
	}

	// Stop the Runner and verify its cancellation result and call count.
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRunnerCoalescesTriggersBeforeSweep(t *testing.T) {
	// Construct a Runner that counts its collection passes.
	runner, _ := newTestRunner(t)

	// Configure collection and coalesce triggers before the Runner starts.
	calls := 0
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		calls++
		return &block_gc.Stats{}, nil
	}
	runner.Trigger()
	runner.Trigger()
	runner.Trigger()

	// Start the Runner and wait for the queued generation to finish.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	// Drain the queued work before stopping the Runner.
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if got := runner.CompletedGeneration(); got != 3 {
		t.Fatalf("completed generation = %d, want 3", got)
	}
}

func TestRunnerDrainWaitsForTriggerDuringDrain(t *testing.T) {
	// Construct a Runner whose first sweep can be held during Drain.
	runner, _ := newTestRunner(t)

	// Gate collection and signal when each sweep begins.
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	calls := 0
	runner.collect = func(ctx context.Context) (*block_gc.Stats, error) {
		calls++
		switch calls {
		case 1:
			close(firstStarted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-firstRelease:
			}
		case 2:
			close(secondStarted)
		}
		return &block_gc.Stats{}, nil
	}

	// Start the Runner before requesting a drain.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	// Begin a sweep and wait until the collector is active.
	runner.Trigger()
	waitSignal(t, firstStarted, "first sweep")

	// Start Drain while the first sweep is blocked.
	drainDone := make(chan error, 1)
	go func() {
		drainDone <- runner.Drain(context.Background())
	}()

	// Trigger another generation and release the active sweep.
	runner.Trigger()
	close(firstRelease)
	waitSignal(t, secondStarted, "second sweep")

	// Require Drain to include the second sweep before stopping the Runner.
	if err := <-drainDone; err != nil {
		t.Fatalf("drain cleanup: %v", err)
	}

	// Cancel the Runner and verify its final error and sweep count.
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRunnerDoesNotCompleteCanceledSweep(t *testing.T) {
	// Construct a Runner whose collector waits for cancellation.
	runner, _ := newTestRunner(t)
	runner.collect = func(ctx context.Context) (*block_gc.Stats, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	// Start the Runner so cancellation interrupts its active sweep.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	// Trigger and cancel the sweep before checking its completion state.
	runner.Trigger()
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if got := runner.CompletedGeneration(); got != 0 {
		t.Fatalf("completed generation = %d, want 0", got)
	}
}

func TestRunnerDoesNotCompleteFailedSweep(t *testing.T) {
	// Construct a Runner whose collector returns a stable error.
	runner, _ := newTestRunner(t)
	wantErr := errors.New("collect failed")
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		return nil, wantErr
	}

	// Run the failed sweep and require its generation to remain incomplete.
	runner.Trigger()
	err := runner.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want collect failed", err)
	}
	if got := runner.CompletedGeneration(); got != 0 {
		t.Fatalf("completed generation = %d, want 0", got)
	}
}

func TestRunnerWaitHonorsContext(t *testing.T) {
	// Construct a Runner with queued work but no active collection.
	runner, _ := newTestRunner(t)
	runner.Trigger()

	// Cancel the wait context before asking the Runner to drain.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
}

func TestRunnerLogsSweepStats(t *testing.T) {
	// Configure a Runner that reports each garbage-collection statistic.
	runner, hook := newTestRunner(t)
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		return &block_gc.Stats{
			NodesSwept:                     3,
			UnreferencedNodeCount:          4,
			RemoveNodeRefsCount:            5,
			RemoveUnreferencedEdgeCount:    6,
			OnSweptCount:                   7,
			RemoveBlockCount:               8,
			Duration:                       9 * time.Millisecond,
			UnreferencedScanDuration:       10 * time.Millisecond,
			RemoveNodeRefsDuration:         11 * time.Millisecond,
			RemoveUnreferencedEdgeDuration: 12 * time.Millisecond,
			OnSweptDuration:                13 * time.Millisecond,
			RemoveBlockDuration:            14 * time.Millisecond,
		}, nil
	}

	// Start the Runner, collect once, and stop it after completion.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()
	runner.Trigger()
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}

	// Verify the Runner emitted one entry with its sweep statistics.
	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("log entry count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Message != "GC swept nodes after provider account cleanup" {
		t.Fatalf("message = %q", entry.Message)
	}

	// Check the log message and the principal collection counts.
	assertLogField(t, entry, "nodes-swept", 3)
	assertLogField(t, entry, "duration", "9ms")
	assertLogField(t, entry, "unreferenced-nodes", 4)
	assertLogField(t, entry, "remove-node-refs", 5)
	assertLogField(t, entry, "remove-unreferenced-edges", 6)
	assertLogField(t, entry, "on-swept-callbacks", 7)

	// Check the durations recorded for each collection phase.
	assertLogField(t, entry, "remove-blocks", 8)
	assertLogField(t, entry, "unreferenced-scan-duration", "10ms")
	assertLogField(t, entry, "remove-node-refs-duration", "11ms")
	assertLogField(t, entry, "remove-unreferenced-edge-duration", "12ms")
	assertLogField(t, entry, "on-swept-duration", "13ms")
	assertLogField(t, entry, "remove-block-duration", "14ms")
}

func TestRunnerSkipsEmptyStatsLog(t *testing.T) {
	// Configure a Runner whose completed sweep has no statistics.
	runner, hook := newTestRunner(t)
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		return &block_gc.Stats{}, nil
	}

	// Start the Runner, await its empty sweep, and verify it remains quiet.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()
	runner.Trigger()
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if entries := hook.AllEntries(); len(entries) != 0 {
		t.Fatalf("log entry count = %d, want 0", len(entries))
	}
}

func newTestRunner(t *testing.T) (*Runner, *test.Hook) {
	// Build a Runner with a test logger and a no-op collector.
	t.Helper()
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.InfoLevel)
	runner := NewRunner(
		logrus.NewEntry(logger).WithField("component", "gc-cleanup-runner"),
		"GC swept nodes after provider account cleanup",
		func(context.Context) (*block_gc.Stats, error) {
			return &block_gc.Stats{}, nil
		},
	)
	return runner, hook
}

func waitSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func assertLogField(t *testing.T, entry *logrus.Entry, key string, want any) {
	t.Helper()
	got, ok := entry.Data[key]
	if !ok {
		t.Fatalf("missing log field %q", key)
	}
	if got != want {
		t.Fatalf("log field %q = %#v, want %#v", key, got, want)
	}
}
