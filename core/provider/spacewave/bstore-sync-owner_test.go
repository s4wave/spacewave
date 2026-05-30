package provider_spacewave

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
	"github.com/sirupsen/logrus"
)

func TestBstoreSyncOwnerRetriesAndRecordsTelemetry(t *testing.T) {
	syncErr := errors.New("sync failed")
	telemetry := &ProviderAccount{}
	sc := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		resourceID: "space-1",
		telemetry:  telemetry,
	}

	calls := make(chan int32, 2)
	var callCount atomic.Int32
	var restarted sync.Once
	restartedCh := make(chan struct{})
	owner := newBstoreSyncOwnerWithRoutine(
		logrus.NewEntry(logrus.New()),
		sc,
		func(ctx context.Context) error {
			call := callCount.Add(1)
			calls <- call
			if call == 1 {
				return syncErr
			}
			restarted.Do(func() { close(restartedCh) })
			<-ctx.Done()
			return ctx.Err()
		},
		routine.WithBackoff(&cbackoff.ZeroBackOff{}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	defer func() {
		cancel()
		owner.Stop()
	}()

	if got := waitForBstoreSyncOwnerCall(t, calls); got != 1 {
		t.Fatalf("first execute call = %d, want 1", got)
	}
	if got := waitForBstoreSyncOwnerCall(t, calls); got != 2 {
		t.Fatalf("second execute call = %d, want 2", got)
	}
	waitForBstoreSyncOwnerSignal(t, restartedCh, time.Second, "sync owner restart")

	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.LastError != syncErr.Error() {
		t.Fatalf("last sync error = %q, want %q", snap.LastError, syncErr.Error())
	}
	if snap.UploadPhase != synctelemetry.UploadPhaseError {
		t.Fatalf("upload phase = %v, want error", snap.UploadPhase)
	}
}

func TestBstoreSyncOwnerStopWaitsForExecuteCleanup(t *testing.T) {
	started := make(chan struct{})
	cleaned := make(chan struct{})
	var startedOnce sync.Once
	owner := newBstoreSyncOwnerWithRoutine(
		logrus.NewEntry(logrus.New()),
		&syncController{},
		func(ctx context.Context) error {
			startedOnce.Do(func() { close(started) })
			defer close(cleaned)
			<-ctx.Done()
			return ctx.Err()
		},
		routine.WithBackoff(&cbackoff.ZeroBackOff{}),
	)

	owner.Start(context.Background())
	waitForBstoreSyncOwnerSignal(t, started, time.Second, "sync owner start")
	owner.Stop()

	select {
	case <-cleaned:
	default:
		t.Fatal("Stop returned before Execute cleanup completed")
	}
}

func waitForBstoreSyncOwnerCall(t *testing.T, calls <-chan int32) int32 {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sync owner execute call")
		return 0
	}
}

func waitForBstoreSyncOwnerSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}
