package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// TestSyncControllerPushPackfileRetriesCanceledPush retries a canceled upload with the same pack.
func TestSyncControllerPushPackfileRetriesCanceledPush(t *testing.T) {
	// Build a controller whose parent context is already canceled.
	s := &syncController{}
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Cancel the first push and accept the retry on a live context.
	calls := 0
	err := s.pushPackfile(parentCtx, "test-pack-id", 7, func(
		ctx context.Context,
		packID string,
		blockCount int,
	) error {
		// Check the push arguments.
		calls++
		if packID != "test-pack-id" {
			t.Fatalf("unexpected pack id %q", packID)
		}
		if blockCount != 7 {
			t.Fatalf("unexpected block count %d", blockCount)
		}
		if calls == 1 {
			return context.Canceled
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("retry context should stay live, got %v", err)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("retry context should have a deadline")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pushPackfile: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 push attempts, got %d", calls)
	}
}

// TestSyncControllerInitSkipsPullForRemoteManifest uses an existing remote manifest without pulling.
func TestSyncControllerInitSkipsPullForRemoteManifest(t *testing.T) {
	// Count the pull requests the server sees.
	ctx := context.Background()
	pullRequests := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/pull") {
			pullRequests <- struct{}{}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Build a controller with a remote manifest and pulls skipped.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	store := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, store)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	remoteEntry := &packfile.PackfileEntry{
		Id:         "remote-pack",
		BlockCount: 1,
		SizeBytes:  128,
	}
	lower := packfile_store.NewPackfileStore(nil, nil, nil)
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      store,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "space-1",
		mfst:       mfst,
		lower:      lower,
		remote: func() []*packfile.PackfileEntry {
			return []*packfile.PackfileEntry{remoteEntry.CloneVT()}
		},
		skipPull: true,
	}

	// Init uses the remote manifest without a pull.
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	select {
	case <-pullRequests:
		t.Fatal("public-read remote init should not call Worker sync/pull")
	default:
	}
	if got := lower.SnapshotStats().ManifestEntries; got != 1 {
		t.Fatalf("lower manifest entries = %d, want 1", got)
	}
}

// TestSyncControllerPullNowRecordsLatestSequenceFromEmptyPull retains sequence progress from empty responses.
func TestSyncControllerPullNowRecordsLatestSequenceFromEmptyPull(t *testing.T) {
	// Serve one empty pull carrying the latest sequence.
	ctx := context.Background()
	respData := mustMarshalVT(t, &packfile.PullResponse{LatestSequence: 9})
	var pullRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bstore/test-res/sync/pull" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		pullRequests++
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Build the controller against the server.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	store := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, store)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      store,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil, nil),
	}

	// Pull once and check the recorded sequence.
	if err := s.PullNow(ctx); err != nil {
		t.Fatalf("PullNow: %v", err)
	}
	if pullRequests != 1 {
		t.Fatalf("pull requests = %d, want 1", pullRequests)
	}
	lastSeq, err := mfst.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatalf("get last pull sequence: %v", err)
	}
	if lastSeq != 9 {
		t.Fatalf("last pull sequence = %d, want 9", lastSeq)
	}
}

// TestSyncControllerPullDuringPush verifies a pull completes while a push is
// still uploading.
func TestSyncControllerPullDuringPush(t *testing.T) {
	// Serve a push that holds until a pull arrives.
	ctx := t.Context()

	// Keep pull and push progress behind independent event gates.
	pushing := make(chan struct{})
	pulled := make(chan struct{})

	// Hold the pack upload until the pull arrives.
	push := newTestPushServer(t, "")
	push.onUpload = func(*packfile.PushRequest) {
		close(pushing)
		select {
		case <-pulled:
		case <-time.After(10 * time.Second):
			t.Error("the pull waited for the push")
		}
	}

	// Answer the pull and route every push request to the push server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/pull") {
			close(pulled)
			return
		}
		push.ServeHTTP(w, r)
	}))
	defer srv.Close()
	push.base = srv.URL

	// Build a syncer against the server.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Use the readable lower store when new packs enter the manifest.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      newSyncTestKvStore(),
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      newSyncTestBlockStore(),
	}

	// Queue one dirty block.
	data := []byte("pull during push")
	ref, _, err := s.upper.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatalf("put block: %v", err)
	}
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}

	// Start the push, then pull while it uploads.
	flushed := make(chan error, 1)
	go func() { err := s.FlushNow(ctx); flushed <- err }()
	select {
	case <-pushing:
	case err := <-flushed:
		t.Fatalf("flush finished before pushing: %v", err)
	}
	if err := s.PullNow(ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if err := <-flushed; err != nil {
		t.Fatalf("flush: %v", err)
	}
}

// TestSyncControllerInitReturnsAccessGatedPullError propagates access failures during initialization.
func TestSyncControllerInitReturnsAccessGatedPullError(t *testing.T) {
	// Encode the access denial.
	ctx := context.Background()
	errResp := &api.ErrorResponse{
		Code:    "rbac_denied",
		Message: "access denied",
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Answer every pull with the denial.
	pullRequests := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count the pull and answer with the denial.
		if !strings.HasSuffix(r.URL.Path, "/sync/pull") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		pullRequests <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Build a syncer against the server.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	store := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, store)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      store,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "space-1",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil, nil),
	}

	// Require Init to return the denial after one pull.
	err = s.Init(ctx)
	if !clouderror.IsAccessGated(err) {
		t.Fatalf("Init() = %v, want access-gated cloud error", err)
	}
	select {
	case <-pullRequests:
	default:
		t.Fatal("expected initial pull request")
	}
	select {
	case <-pullRequests:
		t.Fatal("expected one initial pull request")
	default:
	}
}

// TestSyncControllerExecuteHonorsRetryAfterBackoff delays retries for the server backoff interval.
func TestSyncControllerExecuteHonorsRetryAfterBackoff(t *testing.T) {
	// Encode the retryable rate limit.
	errResp := &api.ErrorResponse{
		Code:      "rate_limited",
		Message:   "retry later",
		Retryable: true,
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Answer every request with the error, counting the requests.
	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count the request.
		select {
		case requests <- struct{}{}:
		default:
		}

		// Answer with the error.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Run a syncer with a dirty block against the server.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	s := newDirtySyncExecuteTestController(t, cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	// Wait for the first push.
	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected initial sync push")
	}

	// Require no retry within the Retry-After interval.
	select {
	case <-requests:
		t.Fatal("retry-after was not honored by dirty sync")
	case <-time.After(syncExecuteNoRetryWindow):
	}

	// Stop the syncer and require a clean exit.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(syncExecuteStopTimeout):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

// TestSyncControllerExecuteBacksOffThroughLocalEdits checks that local edits
// during the retry backoff of a failed flush do not retry it early.
func TestSyncControllerExecuteBacksOffThroughLocalEdits(t *testing.T) {
	// Encode a retryable outage with no Retry-After.
	errResp := &api.ErrorResponse{
		Code:      "unavailable",
		Message:   "try again",
		Retryable: true,
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Answer every request with the error, counting the requests.
	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Run a syncer with a dirty block against the server.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	s := newDirtySyncExecuteTestController(t, cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	// Wait for the first push.
	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected initial sync push")
	}

	// Wake the syncer as local edits do through most of the first 500ms
	// backoff, and require no retry within it.
	edits := time.NewTicker(20 * time.Millisecond)
	defer edits.Stop()
	window := time.After(300 * time.Millisecond)
	for waiting := true; waiting; {
		select {
		case <-requests:
			t.Fatal("a local edit ended the retry backoff")
		case <-edits.C:
			s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				broadcast()
			})
		case <-window:
			waiting = false
		}
	}

	// Stop the syncer and require a clean exit.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(syncExecuteStopTimeout):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

// TestSyncControllerExecuteGatesAccessDeniedFlushFailures waits for access changes after denied writes.
func TestSyncControllerExecuteGatesAccessDeniedFlushFailures(t *testing.T) {
	// Encode the read-only denial.
	errResp := &api.ErrorResponse{
		Code:      "account_read_only",
		Message:   "Account is in a read-only lifecycle state",
		Retryable: false,
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Answer every request with the error, counting the requests.
	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count the request.
		select {
		case requests <- struct{}{}:
		default:
		}

		// Answer with the error.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Run a syncer with a dirty block against the server, gated on the
	// account state.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	gate := &broadcast.Broadcast{}
	s := newDirtySyncExecuteTestController(t, cli, gate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	// Wait for the first push.
	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected initial sync push")
	}

	// Require no retry until the account state changes.
	select {
	case <-requests:
		t.Fatal("gated access denial retried without account state change")
	case <-time.After(syncExecuteNoRetryWindow):
	}

	// Change the account state and require a retry.
	gate.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected account state change to wake gated dirty sync")
	}

	// Stop the syncer and require a clean exit.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(syncExecuteStopTimeout):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

// TestSyncControllerExecuteDrainsPendingWorkOnStop pushes dirty blocks when
// the mount releases before the checkpoint deadline.
func TestSyncControllerExecuteDrainsPendingWorkOnStop(t *testing.T) {
	// Count push requests. Their result does not matter to the drain.
	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Hold the dirty block behind a deadline the test never reaches.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	s := newDirtySyncExecuteTestController(t, cli, nil)
	s.conf = &SyncConfig{SizeThresholdBytes: 1 << 30, CheckpointIntervalSecs: 3600}

	// Release the mount and expect one push before Execute returns.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected Execute to stop after draining")
	}
	if len(requests) == 0 {
		t.Fatal("expected pending dirty blocks to be pushed on stop")
	}
}
