package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/routine"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// TestBstoreSyncOwnerRetriesAndRecordsTelemetry checks that a failed sync
// restarts and records its error in the telemetry.
func TestBstoreSyncOwnerRetriesAndRecordsTelemetry(t *testing.T) {
	// Build a sync controller that records telemetry.
	syncErr := errors.New("sync failed")
	telemetry := &ProviderAccount{}
	sc := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		resourceID: "space-1",
		telemetry:  &telemetry.syncTelemetry,
	}

	// Fail the first sync, then run the restarted one until canceled.
	calls := make(chan int32, 2)
	var callCount atomic.Int32
	var restarted sync.Once
	restartedCh := make(chan struct{})
	owner := newBstoreSyncOwnerWithRoutine(
		logrus.NewEntry(logrus.New()),
		sc,
		func(ctx context.Context) error {
			// Report the call and fail the first one.
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

	// Run the owner for the test.
	ctx, cancel := context.WithCancel(context.Background())
	owner.Start(ctx)
	defer func() {
		cancel()
		owner.Stop()
	}()

	// Wait for the failed sync and its restart.
	if got := waitForBstoreSyncOwnerCall(t, calls); got != 1 {
		t.Fatalf("first execute call = %d, want 1", got)
	}
	if got := waitForBstoreSyncOwnerCall(t, calls); got != 2 {
		t.Fatalf("second execute call = %d, want 2", got)
	}
	waitForBstoreSyncOwnerSignal(t, restartedCh, time.Second, "sync owner restart")

	// The telemetry shows the failure.
	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.LastError != syncErr.Error() {
		t.Fatalf("last sync error = %q, want %q", snap.LastError, syncErr.Error())
	}
	if snap.UploadPhase != synctelemetry.UploadPhaseError {
		t.Fatalf("upload phase = %v, want error", snap.UploadPhase)
	}
}

// TestBstoreSyncOwnerStopWaitsForExecuteCleanup checks that Stop returns
// only after the running sync has cleaned up.
func TestBstoreSyncOwnerStopWaitsForExecuteCleanup(t *testing.T) {
	// Build an owner whose sync runs until canceled.
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

	// Start the sync, then stop it.
	owner.Start(context.Background())
	waitForBstoreSyncOwnerSignal(t, started, time.Second, "sync owner start")
	owner.Stop()

	// The cleanup finished before Stop returned.
	select {
	case <-cleaned:
	default:
		t.Fatal("Stop returned before Execute cleanup completed")
	}
}

// TestSyncControllerCoalescesNonceTriggersIntoPullNow checks that nonce
// triggers during a pull coalesce into one pull, and that a nonce the last
// pull reached starts none.
func TestSyncControllerCoalescesNonceTriggersIntoPullNow(t *testing.T) {
	// Serve pulls that wait for release.
	ctx := context.Background()
	pullStarted := make(chan struct{}, 3)
	releasePull := make(chan struct{})
	var pulls atomic.Uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Answer each pull with the next sequence, starting at 7.
		if r.URL.Path != "/api/bstore/space-1/sync/pull" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		pullStarted <- struct{}{}
		<-releasePull
		seq := 6 + pulls.Add(1)
		_, _ = w.Write(mustMarshalVT(t, &packfile.PullResponse{LatestSequence: seq}))
	}))
	defer srv.Close()

	// Build the sync controller against the server.
	priv, pid := generateTestKeypair(t)
	store := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, store)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	sc := &syncController{
		le:    logrus.NewEntry(logrus.New()),
		store: store,
		client: func() (*SessionClient, error) {
			return NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()), nil
		},
		resourceID: "space-1",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil, nil),
	}

	// Run the remote pull routine for the test.
	sc.remotePullRoutine = newCoalescedTriggerRoutine(
		logrus.NewEntry(logrus.New()),
		"test-bstore-remote-pull",
		sc.pullRemoteOnTrigger,
	)
	runCtx, cancel := context.WithCancel(context.Background())
	sc.remotePullRoutine.SetContext(runCtx)
	defer func() {
		cancel()
		sc.remotePullRoutine.ClearContext()
	}()

	// Trigger again while the first pull runs; no second pull starts yet.
	sc.TriggerRemotePull(1)
	waitForBstoreSyncOwnerSignal(t, pullStarted, time.Second, "nonce-triggered pull")
	sc.TriggerRemotePull(8)
	sc.TriggerRemotePull(8)
	sc.TriggerRemotePull(8)
	select {
	case <-pullStarted:
		t.Fatal("coalesced nonce triggers started a redundant concurrent pull")
	case <-time.After(100 * time.Millisecond):
	}

	// Releasing the pull runs one pending pull.
	close(releasePull)
	waitForBstoreSyncOwnerSignal(t, pullStarted, time.Second, "coalesced pending pull")

	// A nonce the last pull already reached starts no pull.
	sc.TriggerRemotePull(8)
	select {
	case <-pullStarted:
		t.Fatal("a pull started after the coalesced pending pull")
	case <-time.After(250 * time.Millisecond):
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
