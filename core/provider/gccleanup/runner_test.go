//go:build !goscript

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
	runner, _ := newTestRunner(t)

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

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	runner.Trigger()
	waitSignal(t, firstStarted, "first sweep")
	runner.Trigger()
	close(firstRelease)
	waitSignal(t, secondStarted, "second sweep")
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	if got := runner.CompletedGeneration(); got != 2 {
		t.Fatalf("completed generation = %d, want 2", got)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRunnerCoalescesTriggersBeforeSweep(t *testing.T) {
	runner, _ := newTestRunner(t)

	calls := 0
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		calls++
		return &block_gc.Stats{}, nil
	}
	runner.Trigger()
	runner.Trigger()
	runner.Trigger()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

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
	runner, _ := newTestRunner(t)

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

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	runner.Trigger()
	waitSignal(t, firstStarted, "first sweep")
	drainDone := make(chan error, 1)
	go func() {
		drainDone <- runner.Drain(context.Background())
	}()
	runner.Trigger()
	close(firstRelease)
	waitSignal(t, secondStarted, "second sweep")
	if err := <-drainDone; err != nil {
		t.Fatalf("drain cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRunnerDoesNotCompleteCanceledSweep(t *testing.T) {
	runner, _ := newTestRunner(t)
	runner.collect = func(ctx context.Context) (*block_gc.Stats, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

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
	runner, _ := newTestRunner(t)
	wantErr := errors.New("collect failed")
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		return nil, wantErr
	}

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
	runner, _ := newTestRunner(t)
	runner.Trigger()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
}

func TestRunnerLogsSweepStats(t *testing.T) {
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

	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("log entry count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Message != "GC swept nodes after provider account cleanup" {
		t.Fatalf("message = %q", entry.Message)
	}
	assertLogField(t, entry, "nodes-swept", 3)
	assertLogField(t, entry, "duration", "9ms")
	assertLogField(t, entry, "unreferenced-nodes", 4)
	assertLogField(t, entry, "remove-node-refs", 5)
	assertLogField(t, entry, "remove-unreferenced-edges", 6)
	assertLogField(t, entry, "on-swept-callbacks", 7)
	assertLogField(t, entry, "remove-blocks", 8)
	assertLogField(t, entry, "unreferenced-scan-duration", "10ms")
	assertLogField(t, entry, "remove-node-refs-duration", "11ms")
	assertLogField(t, entry, "remove-unreferenced-edge-duration", "12ms")
	assertLogField(t, entry, "on-swept-duration", "13ms")
	assertLogField(t, entry, "remove-block-duration", "14ms")
}

func TestRunnerSkipsEmptyStatsLog(t *testing.T) {
	runner, hook := newTestRunner(t)
	runner.collect = func(context.Context) (*block_gc.Stats, error) {
		return &block_gc.Stats{}, nil
	}

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
