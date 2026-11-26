package provider_spacewave

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	packfile_delta "github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	syncExecuteRequestTimeout = 2 * time.Second
	syncExecuteNoRetryWindow  = 1200 * time.Millisecond
	syncExecuteStopTimeout    = 500 * time.Millisecond
)

// TestSyncPull_Success verifies SyncPull sends a GET to /sync/pull and decodes
// the binary page.
func TestSyncPull_Success(t *testing.T) {
	// Encode one page.
	resp := &packfile.PullResponse{
		Entries: []*packfile.PackfileEntry{
			{Id: "pack-001", BlockCount: 5},
			{Id: "pack-002", BlockCount: 3},
		},
		More: true,
	}
	respData, err := resp.MarshalVT()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	// Serve the page to a GET with no cursor.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request shape.
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/sync/pull") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// Pull the page and compare it.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	page, err := cli.SyncPull(context.Background(), "test-res", 0)
	if err != nil {
		t.Fatalf("SyncPull: %v", err)
	}
	if !page.EqualVT(resp) {
		t.Fatalf("page = %v, want %v", page, resp)
	}
}

// TestSyncPull_WithSince verifies SyncPull adds a since query parameter.
func TestSyncPull_WithSince(t *testing.T) {
	// Require since=5 on the request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")
		if since != "5" {
			t.Errorf("expected since=5, got %q", since)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Pull after cursor 5.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	_, err := cli.SyncPull(context.Background(), "test-res", 5)
	if err != nil {
		t.Fatalf("SyncPull with since: %v", err)
	}
}

// TestSyncPushData verifies SyncPushData admits the pack, uploads its bytes
// to the signed URL, and commits it.
func TestSyncPushData(t *testing.T) {
	// Serve the push protocol for a pack of known bytes.
	packData := []byte("packfile-binary-data-for-testing")
	h := sha256.Sum256(packData)
	bloomFilter := []byte("bloom-filter-bytes")
	push := startTestPushServer(t)

	// Push the pack.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())
	err := cli.SyncPushData(context.Background(), "test-res", "test-pack-id", 42, packData, h[:], bloomFilter, packfile.BloomFormatVersionV1)
	if err != nil {
		t.Fatalf("SyncPushData: %v", err)
	}

	// The cloud committed the admitted pack and its bytes.
	packs := push.committed()
	if len(packs) != 1 {
		t.Fatalf("committed %d packs, want 1", len(packs))
	}
	want := &packfile.PushRequest{
		PackId:             "test-pack-id",
		BlockCount:         42,
		BloomFilter:        bloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		SizeBytes:          uint64(len(packData)),
		Sha256:             h[:],
	}
	if !packs[0].req.EqualVT(want) {
		t.Fatalf("push request = %v, want %v", packs[0].req, want)
	}
	if !bytes.Equal(packs[0].data, packData) {
		t.Fatalf("uploaded %q, want %q", packs[0].data, packData)
	}
}

// TestSyncPushData_AlreadyExists verifies a pack the catalog holds skips the
// upload and the commit.
func TestSyncPushData_AlreadyExists(t *testing.T) {
	// Serve the push protocol, counting uploads.
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)
	bloomFilter := []byte("bloom-filter-bytes")
	push := startTestPushServer(t)
	var uploads atomic.Int32
	push.onUpload = func(*packfile.PushRequest) { uploads.Add(1) }

	// Push the same pack twice.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())
	for range 2 {
		err := cli.SyncPushData(context.Background(), "test-res", "test-pack-id", 3, packData, h[:], bloomFilter, packfile.BloomFormatVersionV1)
		if err != nil {
			t.Fatalf("SyncPushData: %v", err)
		}
	}

	// The second push stopped after admission.
	if n := uploads.Load(); n != 1 {
		t.Fatalf("uploads = %d, want 1", n)
	}
	if packs := push.committed(); len(packs) != 1 {
		t.Fatalf("committed %d packs, want 1", len(packs))
	}
}

// TestSyncPushData_UploadRefused verifies a refused upload fails the push
// without a commit.
func TestSyncPushData_UploadRefused(t *testing.T) {
	// Serve the push protocol.
	packData := []byte("inline-pack-data")
	h := sha256.Sum256([]byte("other-pack-data"))
	push := startTestPushServer(t)

	// Push bytes that do not match the admitted digest.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())
	err := cli.SyncPushData(context.Background(), "test-res", "test-pack-id", 3, packData, h[:], []byte("bloom"), packfile.BloomFormatVersionV1)
	if err == nil || !strings.Contains(err.Error(), "storage refused the upload") {
		t.Fatalf("expected refused upload, got %v", err)
	}

	// Nothing was committed.
	if packs := push.committed(); len(packs) != 0 {
		t.Fatalf("committed %d packs, want 0", len(packs))
	}
}

// TestSyncPushData_MissingBloomFilter rejects uploads without a bloom filter.
func TestSyncPushData_MissingBloomFilter(t *testing.T) {
	// Fail on any request: the client refuses before sending.
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	// Push without a bloom filter and require the refusal.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	err := cli.SyncPushData(
		context.Background(),
		"test-res",
		"test-pack-id",
		3,
		packData,
		h[:],
		nil,
		packfile.BloomFormatVersionV1,
	)
	if err == nil || !strings.Contains(err.Error(), "sync push bloom filter required") {
		t.Fatalf("expected missing bloom filter error, got %v", err)
	}
}

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
		client:     cli,
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
		client:     cli,
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
		client:     cli,
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
		client:     cli,
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
	// Count push requests; their result does not matter to the drain.
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

// syncPackTransport serves packed fixture bytes.
type syncPackTransport struct {
	data []byte
}

// syncErrorTransport returns a configured read failure.
type syncErrorTransport struct {
	err error
}

// syncCountingBlockStore observes upper-store block reads.
type syncCountingBlockStore struct {
	block.StoreOps
	onGet func(*block.BlockRef)
}

// Fetch reads a bounded range of fixture bytes.
func (t *syncPackTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}
	end := min(off+int64(length), int64(len(t.data)))
	return bytes.Clone(t.data[off:end]), nil
}

// Fetch returns the configured failure.
func (t *syncErrorTransport) Fetch(context.Context, int64, int) ([]byte, error) {
	return nil, t.err
}

// GetStoredBlock observes the read before forwarding it.
func (s *syncCountingBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if s.onGet != nil {
		s.onGet(ref)
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// syncTestBlockStore stands in for the GC-backed bucket handle, which knows
// every block's refs. Test blocks carry none, so each reads as a leaf.
type syncTestBlockStore struct {
	block.StoreOps
}

// GetStoredBlock returns the block's bytes with an empty, known ref list.
func (s syncTestBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := block.GetBlockWithoutRefs(ctx, s.StoreOps, ref)
	if stored != nil {
		stored.RefsSource = block.RefsAsserted
	}
	return stored, err
}

// newSyncTestBlockStore constructs the provider block test store.
func newSyncTestBlockStore() block.StoreOps {
	return syncTestBlockStore{StoreOps: newProviderSpacewaveTestBlockStore(hash.RecommendedHashType)}
}

// newSyncTestLowerPackfileStore packs fixture blocks into a readable lower store.
func newSyncTestLowerPackfileStore(t *testing.T, blocks map[string][]byte) *packfile_store.PackfileStore {
	// Hash each fixture block.
	t.Helper()
	var items []struct {
		h    *hash.Hash
		data []byte
	}
	for _, data := range blocks {
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatalf("sum lower block hash: %v", err)
		}
		items = append(items, struct {
			h    *hash.Hash
			data []byte
		}{h: h, data: data})
	}

	// Pack the blocks in order.
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		item := items[idx]
		idx++
		return item.h, &block.StoredBlock{Data: item.data, RefsSource: block.RefsAsserted}, nil
	})
	if err != nil {
		t.Fatalf("pack lower blocks: %v", err)
	}

	// Serve the pack from memory.
	packData := bytes.Clone(buf.Bytes())
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncPackTransport{data: packData},
			), nil
		},
		nil,
		nil,
	)

	// Make the failing pack a candidate for every requested block.
	lower.UpdateManifest([]*packfile.PackfileEntry{{
		Id:                 "existing-pack",
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
	}})
	return lower
}

// newSyncTestErrorLowerPackfileStore constructs a lower store whose reads fail.
func newSyncTestErrorLowerPackfileStore() *packfile_store.PackfileStore {
	// Fail every transport read from the lower store.
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncErrorTransport{err: io.ErrUnexpectedEOF},
			), nil
		},
		nil, nil)
	lower.UpdateManifest([]*packfile.PackfileEntry{{
		Id:                 "error-pack",
		BloomFilter:        []byte("invalid-bloom"),
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         1,
		SizeBytes:          1,
	}})
	return lower
}

// syncTestLists is a fake of the child lists a Volume keeps, keyed by the
// block's string form.
type syncTestLists map[string][]*block.BlockRef

// GetChildren returns the children listed for ref.
func (l syncTestLists) GetChildren(_ context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error) {
	children, ok := l[ref.MarshalString()]
	return slices.Clone(children), ok, nil
}

// newSyncTestKvStore constructs a transactional in-memory store.
func newSyncTestKvStore() kvtx.Store {
	return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
}

// assertSyncPackEntryMetadata checks uploaded pack discovery metadata.
func assertSyncPackEntryMetadata(t *testing.T, entries []*packfile.PackfileEntry) {
	t.Helper()
	for i, entry := range entries {
		if entry.GetBlockCount() == 0 {
			t.Fatalf("entry %d missing block count", i)
		}
		if len(entry.GetBloomFilter()) == 0 {
			t.Fatalf("entry %d missing bloom filter", i)
		}
		if entry.GetCreatedAt() == nil {
			t.Fatalf("entry %d missing created-at metadata", i)
		}
	}
}

// readPackPhysicalKeys returns block keys in physical pack order.
func readPackPhysicalKeys(t *testing.T, body []byte) []string {
	// Read the pack index.
	t.Helper()
	reader, err := kvfile.BuildReader(bytes.NewReader(body), uint64(len(body)))
	if err != nil {
		t.Fatalf("build pack reader: %v", err)
	}

	// Collect the index entries in offset order.
	var entries []*kvfile.IndexEntry
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		entries = append(entries, ie.CloneVT())
		return nil
	})
	if err != nil {
		t.Fatalf("scan pack entries: %v", err)
	}
	slices.SortFunc(entries, func(a, b *kvfile.IndexEntry) int {
		return cmp.Compare(a.GetOffset(), b.GetOffset())
	})

	// Decode each entry's block key.
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		h, err := packfile.ParseBlockKey(entry.GetKey())
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, h.MarshalString())
	}
	return keys
}

// addSyncDirtyBlock stores a block and queues it for upload.
func addSyncDirtyBlock(
	t *testing.T,
	ctx context.Context,
	wtx kvtx.Tx,
	upper block.StoreOps,
	body string,
) *block.BlockRef {
	// Store the block and mark it for upload.
	t.Helper()
	data := []byte(body)
	ref, _, err := upper.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatalf("put upper block: %v", err)
	}
	if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
		t.Fatalf("set dirty key: %v", err)
	}
	return ref
}

// newDirtySyncExecuteTestController constructs a controller with pending upload work.
func newDirtySyncExecuteTestController(
	t *testing.T,
	cli *SessionClient,
	gate *broadcast.Broadcast,
) *syncController {
	// Build the dirty and manifest stores.
	t.Helper()
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Queue one dirty block.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Build the controller over the stores.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil, nil),
		upper:      upper,
		conf:       &SyncConfig{SizeThresholdBytes: 1, CheckpointIntervalSecs: 1},
		gateBcast:  gate,
	}
	s.updateDirtyState(ctx)
	return s
}

// syncTestChunkBlockBytes sizes test blocks so two fill one sync pack and a
// third starts the next.
const syncTestChunkBlockBytes = int(syncFlushMaxPackBytes/2) - 64*1024

// countSyncDirtyKeys returns the number of dirty markers in store.
func countSyncDirtyKeys(t *testing.T, ctx context.Context, store kvtx.Store) int {
	// Scan the dirty prefix in a read transaction.
	t.Helper()
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	var count int
	if err := rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	return count
}

// addSyncChunkBlocks stores n distinct blocks of syncTestChunkBlockBytes in
// upper and marks them dirty in store.
func addSyncChunkBlocks(t *testing.T, ctx context.Context, store kvtx.Store, upper block.StoreOps, n int) {
	// Open the dirty marker transaction.
	t.Helper()
	wtx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Store and mark each block.
	for i := range n {
		data := bytes.Repeat([]byte{byte(i + 1)}, syncTestChunkBlockBytes)
		ref, _, err := upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatalf("put upper block: %v", err)
		}
		if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}

	// Commit the markers.
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}
}

// TestSyncControllerFlushChunksLargeDirtySet uploads bounded packs before
// reading all dirty data, and packs the next chunk while the first uploads.
func TestSyncControllerFlushChunksLargeDirtySet(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Count block reads; secondChunkLoaded closes once the packer has loaded
	// the second chunk.
	const blockCount = 6
	var getCount, firstPushGetCount atomic.Int64
	secondChunkLoaded := make(chan struct{})
	var secondChunkOnce, firstUpload sync.Once
	upper := &syncCountingBlockStore{
		StoreOps: newSyncTestBlockStore(),
		onGet: func(*block.BlockRef) {
			if getCount.Add(1) == 4 {
				secondChunkOnce.Do(func() { close(secondChunkLoaded) })
			}
		},
	}
	addSyncChunkBlocks(t, ctx, dirtyStore, upper, blockCount)

	// Hold the first upload until the second chunk is packed.
	push := startTestPushServer(t)
	push.onUpload = func(*packfile.PushRequest) {
		firstUpload.Do(func() {
			select {
			case <-secondChunkLoaded:
			case <-time.After(10 * time.Second):
				t.Error("the second chunk was not packed during the first upload")
			}
			firstPushGetCount.Store(getCount.Load())
		})
	}

	// Flush the dirty set through a controller.
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String()),
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// The flush pushed three packs, the first before reading every block.
	pushed := push.committed()
	if len(pushed) != 3 {
		t.Fatalf("sync pushes = %d, want 3", len(pushed))
	}
	if firstPushGetCount.Load() >= blockCount {
		t.Fatalf("expected first push before all dirty blocks loaded, loaded %d", firstPushGetCount.Load())
	}

	// Every pack fits the pack and wire limits.
	for i, pack := range pushed {
		size := len(pack.data)
		if int64(size) > syncFlushMaxPackBytes {
			t.Fatalf("push %d exceeded the sync pack target: %d", i, size)
		}
		if int64(size) > packfile_delta.DefaultMaxChunkBytes {
			t.Fatalf("push %d exceeded wire chunk limit: %d", i, size)
		}
	}

	// The manifest holds every pack and the dirty set is empty.
	if len(mfst.GetEntries()) != len(pushed) {
		t.Fatalf("manifest entries = %d, want %d", len(mfst.GetEntries()), len(pushed))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}
}

// TestSyncControllerFlushCommitsPushedPacksBeforeFailure keeps the packs pushed
// before a failed push in the manifest and clears their dirty markers.
func TestSyncControllerFlushCommitsPushedPacksBeforeFailure(t *testing.T) {
	// Mark six chunk-sized blocks dirty.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	const blockCount = 6
	upper := newSyncTestBlockStore()
	addSyncChunkBlocks(t, ctx, dirtyStore, upper, blockCount)

	// Refuse the third push.
	push := newTestPushServer(t, "")
	var pushCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			pushCount++
			if pushCount > 2 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		push.ServeHTTP(w, r)
	}))
	defer srv.Close()
	push.base = srv.URL

	// Flush the dirty set until the refusal.
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}

	// Drain the queue through the production packer and HTTP upload.
	if _, err := s.flush(ctx, false); err == nil {
		t.Fatal("expected flush to fail on the third push")
	}

	// The manifest kept both committed packs.
	if got := len(mfst.GetEntries()); got != 2 {
		t.Fatalf("manifest entries = %d, want the 2 pushed packs", got)
	}

	// Only the blocks outside the committed packs stay dirty.
	var pushedBlocks int
	for _, pack := range push.committed() {
		pushedBlocks += int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
	}
	want := blockCount - pushedBlocks
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != want {
		t.Fatalf("dirty entries = %d, want %d unpushed blocks", n, want)
	}

	// The persisted summary counts the same blocks and keeps the first deadline.
	state := readPersistedPending(t, dirtyStore)
	if int(state.GetCount()) != want || state.GetSizeBytes() != int64(want*syncTestChunkBlockBytes) || state.GetPendingSinceNanos() == 0 {
		t.Fatalf("persisted queue %v, want %d blocks of %d bytes", state, want, syncTestChunkBlockBytes)
	}
}

// TestSyncControllerFlushDedupesLowerBlocks filters stored blocks before reading upper data.
func TestSyncControllerFlushDedupesLowerBlocks(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark a duplicate and a fresh block dirty.
	baseUpper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	duplicate := addSyncDirtyBlock(t, ctx, wtx, baseUpper, "duplicate dirty block")
	fresh := addSyncDirtyBlock(t, ctx, wtx, baseUpper, "fresh dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Record whether the flush reads the duplicate from the upper store.
	var duplicateFetched atomic.Bool
	upper := &syncCountingBlockStore{
		StoreOps: baseUpper,
		onGet: func(ref *block.BlockRef) {
			if ref.GetHash().MarshalString() == duplicate.GetHash().MarshalString() {
				duplicateFetched.Store(true)
			}
		},
	}

	// Flush against a lower store that already holds the duplicate.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower: newSyncTestLowerPackfileStore(t, map[string][]byte{
			"duplicate": []byte("duplicate dirty block"),
		}),
		upper: upper,
	}
	telemetry := &ProviderAccount{}
	s.telemetry = &telemetry.syncTelemetry
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if duplicateFetched.Load() {
		t.Fatal("expected lower-existing duplicate to be filtered before reading upper data")
	}

	// Only the fresh block was packed and the dirty set is empty.
	if len(mfst.GetEntries()) != 1 {
		t.Fatalf("expected 1 manifest entry, got %d", len(mfst.GetEntries()))
	}
	want := []string{fresh.GetHash().MarshalString()}
	if got := readPackPhysicalKeys(t, testPushedBody(t, push)); !slices.Equal(got, want) {
		t.Fatalf("physical pack keys = %v, want %v; duplicate=%s", got, want, duplicate.GetHash().MarshalString())
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}

	// Telemetry counts one upload and one deduped block.
	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.PushCount != 1 || snap.PushedBytes == 0 {
		t.Fatalf("expected one uploaded pack in telemetry, got %+v", snap)
	}
	if snap.DedupedUploadCount != 1 || snap.DedupedUploadBytes != int64(len("duplicate dirty block")) {
		t.Fatalf("unexpected dedup telemetry: %+v", snap)
	}
	if snap.PendingUploadBytes != 0 || snap.PendingUploadCount != 0 {
		t.Fatalf("expected no pending upload after cleanup, got %+v", snap)
	}
}

// TestSyncControllerFlushAllDuplicateDirtyBlocksSkipsPush clears duplicate work without uploading.
func TestSyncControllerFlushAllDuplicateDirtyBlocksSkipsPush(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Count every request to the cloud.
	var pushCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark two blocks the lower store already holds dirty.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "duplicate-a")
	addSyncDirtyBlock(t, ctx, wtx, upper, "duplicate-b")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush the dirty set.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower: newSyncTestLowerPackfileStore(t, map[string][]byte{
			"a": []byte("duplicate-a"),
			"b": []byte("duplicate-b"),
		}),
		upper: upper,
	}
	telemetry := &ProviderAccount{}
	s.telemetry = &telemetry.syncTelemetry
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Nothing was pushed and the dirty set is empty.
	if pushCount != 0 {
		t.Fatalf("expected no sync pushes, got %d", pushCount)
	}
	if len(mfst.GetEntries()) != 0 {
		t.Fatalf("expected no manifest entries, got %d", len(mfst.GetEntries()))
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}

	// Telemetry counts both blocks as deduped.
	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.PushCount != 0 || snap.PushedBytes != 0 {
		t.Fatalf("expected no uploaded pack telemetry, got %+v", snap)
	}
	if snap.DedupedUploadCount != 2 ||
		snap.DedupedUploadBytes != int64(len("duplicate-a")+len("duplicate-b")) {
		t.Fatalf("unexpected dedup telemetry: %+v", snap)
	}
	if snap.PendingUploadBytes != 0 || snap.PendingUploadCount != 0 {
		t.Fatalf("expected no pending upload after all-duplicate cleanup, got %+v", snap)
	}
}

// TestSyncControllerFlushDuplicateProbeErrorPreservesDirty retains dirty work when deduplication fails.
func TestSyncControllerFlushDuplicateProbeErrorPreservesDirty(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Mark one block dirty.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, "https://example.invalid", DefaultSigningEnvPrefix, priv, pid.String())
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush against a lower store whose probe fails.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      newSyncTestErrorLowerPackfileStore(),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err == nil {
		t.Fatal("expected duplicate probe error")
	}

	// The dirty block remains for the next flush.
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 1 {
		t.Fatalf("expected dirty key to remain after probe error, got %d", n)
	}
}

// TestSyncControllerFlushOrdersBlocksByGCGraph writes parent blocks before their children.
func TestSyncControllerFlushOrdersBlocksByGCGraph(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Open a dirty transaction over the upper store.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Mark four blocks dirty out of graph order.
	stray := addSyncDirtyBlock(t, ctx, wtx, upper, "stray")
	childA := addSyncDirtyBlock(t, ctx, wtx, upper, "child-a")
	rootB := addSyncDirtyBlock(t, ctx, wtx, upper, "root-b")
	rootA := addSyncDirtyBlock(t, ctx, wtx, upper, "root-a")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// List child-a under root-a.
	graph := syncTestLists{rootA.MarshalString(): {childA}}

	// Flush in subtree order.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
		childLists: graph,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Each root's children follow it at once, and every block is packed.
	got := readPackPhysicalKeys(t, testPushedBody(t, push))
	i := slices.Index(got, rootA.GetHash().MarshalString())
	if len(got) != 4 || i < 0 || i+1 >= len(got) || got[i+1] != childA.GetHash().MarshalString() {
		t.Fatalf("physical pack order = %v, want child-a right after root-a", got)
	}
	for _, ref := range []*block.BlockRef{rootB, stray} {
		if !slices.Contains(got, ref.GetHash().MarshalString()) {
			t.Fatalf("physical pack order = %v, missing %s", got, ref.MarshalString())
		}
	}
}

// TestSyncControllerFlushChunksBlockCountCeiling splits packs at the wire block limit.
func TestSyncControllerFlushChunksBlockCountCeiling(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark one block more than a pack may hold dirty.
	blockCount := int(writer.DefaultMaxBlocksPerPack) + 1

	// Store blocks and their upload markers in one committed queue.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Fill the queue with distinct payloads under the same transaction.
	for i := range blockCount {
		data := []byte("small dirty block " + strconv.Itoa(i))
		ref, _, err := upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatalf("put upper block: %v", err)
		}
		if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush the dirty set.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// The flush split the blocks across two packs within the ceiling.
	pushed := push.committed()
	if len(pushed) != 2 {
		t.Fatalf("expected 2 sync pushes, got %d", len(pushed))
	}
	total := 0
	for i, pack := range pushed {
		if len(pack.req.GetBloomFilter()) == 0 {
			t.Fatal("missing bloom filter")
		}
		count := int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
		if count > int(writer.DefaultMaxBlocksPerPack) {
			t.Fatalf("push %d exceeded block ceiling: %d", i, count)
		}
		total += count
	}

	// Every block was pushed and the manifest holds both packs.
	if total != blockCount {
		t.Fatalf("pushed block total = %d, want %d", total, blockCount)
	}
	if len(mfst.GetEntries()) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(mfst.GetEntries()))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())
}

// corruptUpperStore reads one block as corrupt, as an upper store whose disk
// returned the wrong bytes does, and serves the rest.
type corruptUpperStore struct {
	block.StoreOps
	bad *hash.Hash
}

func (s *corruptUpperStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.GetHash().EqualVT(s.bad) {
		return nil, &block.CorruptError{Ref: ref}
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// TestSyncControllerFlushSkipsCorruptDirtyBlock puts a dirty block whose local
// bytes fail their hash among good ones: the good ones upload and clear, the
// corrupt one stays dirty and counted, the flush reports it held back as a
// count, not an error, and the upper store keeps every block.
func TestSyncControllerFlushSkipsCorruptDirtyBlock(t *testing.T) {
	// Mark three blocks dirty and read the second as corrupt.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	const blockCount = 3
	inner := newSyncTestBlockStore()
	addSyncChunkBlocks(t, ctx, dirtyStore, inner, blockCount)
	var refs []*block.BlockRef
	for i := range blockCount {
		ref, err := block.BuildBlockRef(bytes.Repeat([]byte{byte(i + 1)}, syncTestChunkBlockBytes), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	upper := &corruptUpperStore{StoreOps: inner, bad: refs[1].GetHash()}

	// Flush through the production packer and HTTP upload.
	push := newTestPushServer(t, "")
	srv := httptest.NewServer(push)
	defer srv.Close()
	push.base = srv.URL
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if corrupt, err := s.flush(ctx, false); err != nil || corrupt != 1 {
		t.Fatalf("flush = %d corrupt, %v; want 1, nil", corrupt, err)
	}

	// The two good blocks uploaded, and no pushed pack holds the corrupt one.
	var pushedBlocks int
	for _, pack := range push.committed() {
		pushedBlocks += int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
		if slices.Contains(readPackPhysicalKeys(t, pack.data), refs[1].GetHash().MarshalString()) {
			t.Fatal("pushed pack holds the corrupt block")
		}
	}
	if pushedBlocks != blockCount-1 {
		t.Fatalf("pushed %d blocks, want %d", pushedBlocks, blockCount-1)
	}

	// Only the corrupt block stays dirty, and the queue still counts it.
	pending, err := s.scanDirtyCandidates(ctx)
	if err != nil || len(pending) != 1 || !pending[0].hash.EqualVT(refs[1].GetHash()) {
		t.Fatalf("dirty after flush = %d blocks, %v; want only the corrupt block", len(pending), err)
	}
	if state := readPersistedPending(t, dirtyStore); state.GetCount() != 1 {
		t.Fatalf("persisted queue counts %d blocks, want 1", state.GetCount())
	}

	// Nothing was freed: the upper store holds every block.
	for i, ref := range refs {
		found, err := inner.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("block %d after flush: found = %v, err = %v", i, found, err)
		}
	}

	// A second flush skips it again and still uploads nothing twice.
	if corrupt, err := s.flush(ctx, false); err != nil || corrupt != 1 {
		t.Fatalf("second flush = %d corrupt, %v; want 1, nil", corrupt, err)
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 1 {
		t.Fatalf("dirty entries after second flush = %d, want 1", n)
	}
}

// TestSyncControllerHeldWaitIgnoresFlushBroadcast checks that a held store
// waits for a new write or its recheck deadline, not for the broadcast its own
// flush raised, so it does no flush while idle. A write of the held hash
// clears it from the known set.
func TestSyncControllerHeldWaitIgnoresFlushBroadcast(t *testing.T) {
	// Hold the only dirty block as corrupt.
	s := newDirtySyncExecuteTestController(t, nil, nil)
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: s.upper, bad: ref.GetHash()}
	writes, _ := s.writeSnapshot()
	var held *heldError
	if err := s.FlushNow(t.Context()); !errors.As(err, &held) {
		t.Fatalf("flush = %v; want held", err)
	}

	// The flush's own broadcast and any later one leave the wait to its
	// deadline, which reports a recheck.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	start := time.Now()
	recheck, err := s.waitHeld(t.Context(), writes, start.Add(200*time.Millisecond), time.Hour)
	if err != nil || !recheck || time.Since(start) < 200*time.Millisecond {
		t.Fatalf("wait = %v, %v after %v; want a recheck at the deadline", recheck, err, time.Since(start))
	}

	// A write ends the wait one interval later, without a recheck, and
	// clears the written hash from the known set.
	writes, _ = s.writeSnapshot()
	go func() {
		time.Sleep(20 * time.Millisecond)
		if err := s.MarkDirty(context.Background(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
			t.Error(err)
		}
	}()
	recheck, err = s.waitHeld(t.Context(), writes, time.Now().Add(time.Hour), 10*time.Millisecond)
	if err != nil || recheck {
		t.Fatalf("wait after write = %v, %v; want a flush without a recheck", recheck, err)
	}
	if hasKey(s.hold.blocks, ref.GetHash().MarshalString()) {
		t.Fatal("write of the held hash left it known corrupt")
	}
}

// pausingReadStore pauses its pauseAt-th read transaction until release
// closes, so a test can act between two reads of one flush.
type pausingReadStore struct {
	kvtx.Store
	reads   atomic.Int64
	pauseAt atomic.Int64
	paused  chan struct{}
	release chan struct{}
}

// NewTransaction pauses the armed read transaction.
func (s *pausingReadStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		if n := s.reads.Add(1); n == s.pauseAt.Load() {
			close(s.paused)
			<-s.release
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// TestSyncControllerHeldWriteDuringFlushRereads writes a held hash, after the
// block was removed and written again, while a flush is paused past its copy
// of the known set. That flush read nothing and holds the block, but the next
// flush must read it, upload it and clear it, not wait for the recheck
// deadline.
func TestSyncControllerHeldWriteDuringFlushRereads(t *testing.T) {
	// Accept every push.
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			uploads.Add(1)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, server.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Hold the only dirty block as corrupt.
	s := newDirtySyncExecuteTestController(t, cli, nil)
	good := s.upper
	ref, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: good, bad: ref.GetHash()}
	store := &pausingReadStore{Store: s.store, paused: make(chan struct{}), release: make(chan struct{})}
	s.store = store
	var held *heldError
	if err := s.FlushNow(t.Context()); !errors.As(err, &held) {
		t.Fatalf("first flush = %v; want held", err)
	}

	// Pause the next flush at its first page read, after its copy of the
	// known set, and rewrite the block meanwhile.
	store.pauseAt.Store(store.reads.Load() + 2)
	flushed := make(chan error, 1)
	go func() { flushed <- s.FlushNow(t.Context()) }()
	select {
	case <-store.paused:
	case err := <-flushed:
		t.Fatalf("flush ended before its pause: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("flush never reached its pause")
	}
	if err := good.RmBlock(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := good.PutBlock(t.Context(), []byte("dirty block"), nil); err != nil {
		t.Fatal(err)
	}
	s.upper = good
	if err := s.MarkDirty(t.Context(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	close(store.release)
	if err := <-flushed; !errors.As(err, &held) {
		t.Fatalf("paused flush = %v; want held from its copy", err)
	}
	if uploads.Load() != 0 {
		t.Fatalf("uploads before the rewrite was read = %d, want 0", uploads.Load())
	}

	// The next flush reads the rewritten block, uploads it and clears it.
	if err := s.FlushNow(t.Context()); err != nil {
		t.Fatalf("flush after the write = %v; want the block read and uploaded", err)
	}
	if uploads.Load() == 0 {
		t.Fatal("the rewritten block was not uploaded")
	}
	if pending, err := s.scanDirtyCandidates(t.Context()); err != nil || len(pending) != 0 {
		t.Fatalf("dirty after the flush = %d blocks, %v; want none", len(pending), err)
	}
}

// heldTestController builds a sync controller over a server that fails every
// push from the failFrom-th on while failFrom is not zero, with telemetry, so
// a test reads the held state the sync status shows. Its queue holds the
// "dirty block" first, then five chunk-sized blocks that fill three chunks.
// The upper store reads the hashes in bad as corrupt.
func heldTestController(t *testing.T) (*syncController, *atomic.Int32, *atomic.Int32, *corruptSetStore) {
	// Fail pushes from failFrom on and count them.
	var failFrom, pushes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			if n, f := pushes.Add(1), failFrom.Load(); f != 0 && n >= f {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, server.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Queue the chunk-sized blocks behind the dirty block and read through
	// the corrupt set.
	s := newDirtySyncExecuteTestController(t, cli, nil)
	s.telemetry = &NewTestProviderAccount(t, server.URL).syncTelemetry
	addSyncChunkBlocks(t, t.Context(), s.store, s.upper, 5)
	s.updateDirtyState(t.Context())
	upper := &corruptSetStore{StoreOps: s.upper, bad: map[string]struct{}{}}
	s.upper = upper
	return s, &failFrom, &pushes, upper
}

// corruptSetStore reads each block whose hash is in bad as corrupt.
type corruptSetStore struct {
	block.StoreOps
	bad map[string]struct{}
}

func (s *corruptSetStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if hasKey(s.bad, ref.GetHash().MarshalString()) {
		return nil, &block.CorruptError{Ref: ref}
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// heldStatus returns the held state the sync status shows for the test store.
func heldStatus(t *testing.T, s *syncController) (int, bool, time.Time) {
	t.Helper()
	for _, b := range s.telemetry.Snapshot().BlockStores {
		if b.BlockStoreID == s.resourceID {
			return b.CorruptDirtyCount, b.HeldPartial, b.HeldSince
		}
	}
	return 0, false, time.Time{}
}

// wantHold checks the controller's held set and the status it shows.
func wantHold(t *testing.T, stage string, s *syncController, keys []string, partial bool) {
	t.Helper()
	var hold holdState
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { hold = s.hold })
	count, shownPartial, since := heldStatus(t, s)
	held := len(keys) != 0
	if len(hold.blocks) != len(keys) || hold.partial != partial || hold.since.IsZero() == held ||
		count != len(keys) || shownPartial != partial || !since.Equal(hold.since) {
		t.Fatalf("%s: held %d partial %v since %v, shown %d partial %v since %v; want %d held, partial %v",
			stage, len(hold.blocks), hold.partial, hold.since, count, shownPartial, since, len(keys), partial)
	}
	for _, key := range keys {
		if !hasKey(hold.blocks, key) {
			t.Fatalf("%s: a block that must stay held is not", stage)
		}
	}
}

// TestSyncControllerPartialPassKeepsHeld fails the push of a flush's first
// chunk, so the flush stops before reading every dirty block. The blocks it
// found corrupt are held with the count shown partial, a hold on a block it
// did not reach is kept, and a later complete flush replaces the set and
// clears partial. The flushes are unordered so the chunks follow the dirty
// sequence.
func TestSyncControllerPartialPassKeepsHeld(t *testing.T) {
	// Queue a late block behind the chunks and read the dirty block corrupt.
	ctx := t.Context()
	s, failFrom, pushes, upper := heldTestController(t)
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	late := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "late block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s.updateDirtyState(ctx)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey, lateKey := bad.GetHash().MarshalString(), late.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}

	// The first held flush fails its first push: held, partial, at least
	// the block it found.
	failFrom.Store(1)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("failing flush = %v; want the push error", err)
	}
	wantHold(t, "after a failed push", s, []string{badKey}, true)
	var since time.Time
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { since = s.hold.since })

	// An earlier hold on the late block survives a rechecking flush that
	// fails before reaching it, with the first held time.
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{badKey: {}, lateKey: {}}, false)
	})
	s.corruptRecheck.Store(true)
	pushes.Store(0)
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("second failing flush = %v; want the push error", err)
	}
	wantHold(t, "after a partial pass", s, []string{badKey, lateKey}, true)
	var since2 time.Time
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { since2 = s.hold.since })
	if !s.corruptRecheck.Load() || !since2.Equal(since) {
		t.Fatal("the partial pass lost the recheck or restarted the held time")
	}

	// A complete flush that reads every block replaces the set and clears
	// partial: the late block reads sound and uploads.
	failFrom.Store(0)
	if err := s.FlushNowUnordered(ctx); !errors.As(err, &held) || held.count != 1 {
		t.Fatalf("complete flush = %v; want held with exactly 1 block", err)
	}
	wantHold(t, "after a complete pass", s, []string{badKey}, false)
}

// TestSyncControllerWriteClearsPartialHold holds a block on a flush that
// fails its first push, then writes the block sound. The write empties the
// held set, so the store is not held and the status shows nothing held, and a
// flush that fails again before finding anything leaves it so.
func TestSyncControllerWriteClearsPartialHold(t *testing.T) {
	// Hold the dirty block on a failed flush.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}
	failFrom.Store(1)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after a failed push", s, []string{badKey}, true)

	// Remove the block and write it sound: nothing is held.
	delete(upper.bad, badKey)
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: bad.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	wantHold(t, "after the write", s, nil, false)

	// A flush that fails before finding anything keeps it not held.
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after another failed push", s, nil, false)
}

// wantMarker checks that the dirty marker and the stored bytes of a damaged
// block are still there.
func wantMarker(t *testing.T, stage string, s *syncController, upper *corruptSetStore, ref *block.BlockRef) {
	t.Helper()
	var marked bool
	err := kvtx.RunTransaction(t.Context(), false,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			marked, err = tx.Exists(ctx, []byte("dirty/"+ref.GetHash().MarshalString()))
			return err
		},
	)
	if err != nil || !marked {
		t.Fatalf("%s: the damaged block's marker is gone: %v", stage, err)
	}
	if found, err := upper.GetBlockExists(t.Context(), ref); err != nil || !found {
		t.Fatalf("%s: the damaged block's bytes are gone: %v", stage, err)
	}
}

// TestSyncControllerPartialPassDropsSoundHeld holds a block that has since
// read sound, with its recheck due, and damages a block in the same first
// chunk. The flush reads both, pushes and commits that chunk, then fails the
// next push. The block it proved sound must not be counted as held; the
// damaged block stays held, partial, with its marker and bytes.
func TestSyncControllerPartialPassDropsSoundHeld(t *testing.T) {
	// Hold the dirty block, sound on disk, with a recheck due, and damage the
	// first chunk-sized block.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	sound, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := block.BuildBlockRef(bytes.Repeat([]byte{1}, syncTestChunkBlockBytes), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{sound.GetHash().MarshalString(): {}}, false)
	})
	s.corruptRecheck.Store(true)

	// The second push fails after the first chunk committed.
	failFrom.Store(2)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantHold(t, "after the partial pass", s, []string{badKey}, true)
	wantMarker(t, "after the partial pass", s, upper, bad)
}

// TestSyncControllerHeldMarkerSurvives tries to lose a damaged block's dirty
// marker across the paths that change the held state: a failed flush, a
// write of the damaged hash that keeps its damaged bytes and empties the
// held set, a write of another block, and a complete flush. The marker and
// the bytes must stay, and the last flush must hold exactly that block.
// TestSyncControllerPartialPassDropsSoundHeld covers a flush that fails after
// a committed chunk.
func TestSyncControllerHeldMarkerSurvives(t *testing.T) {
	// Read the dirty block as damaged throughout.
	ctx := t.Context()
	s, failFrom, _, upper := heldTestController(t)
	bad, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	badKey := bad.GetHash().MarshalString()
	upper.bad[badKey] = struct{}{}

	// A failed flush holds it.
	failFrom.Store(1)
	if err := s.FlushNowUnordered(ctx); err == nil {
		t.Fatal("failing flush succeeded")
	}
	wantMarker(t, "failed flush", s, upper, bad)
	wantHold(t, "failed flush", s, []string{badKey}, true)

	// A write that keeps the damaged bytes empties the held set.
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: bad.GetHash(), Size: int64(len("dirty block"))}}); err != nil {
		t.Fatal(err)
	}
	wantMarker(t, "write of the damaged hash", s, upper, bad)
	wantHold(t, "write of the damaged hash", s, nil, false)

	// A write of another block, then a complete flush that uploads every
	// sound block and holds the damaged one again.
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	other := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "other block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: other.GetHash(), Size: int64(len("other block"))}}); err != nil {
		t.Fatal(err)
	}
	failFrom.Store(0)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); !errors.As(err, &held) || held.count != 1 {
		t.Fatalf("complete flush = %v; want exactly the damaged block held", err)
	}
	wantMarker(t, "complete flush", s, upper, bad)
	if n := readPersistedPending(t, s.store).GetCount(); n != 1 {
		t.Fatalf("pending markers = %d; want only the damaged block", n)
	}
	wantHold(t, "complete flush", s, []string{badKey}, false)
}

// TestSyncControllerFailedPushKeepsRecheck holds a block that a write has
// since made sound, with its recheck pending, in a chunk after the first. A
// flush that consumes the recheck and fails its first push stops before that
// chunk; it must hand the recheck back so the next flush reads the block and
// uploads it rather than holding it unread.
func TestSyncControllerFailedPushKeepsRecheck(t *testing.T) {
	// Queue the held block, sound on disk, known and due a recheck, behind
	// the chunks. The flushes are unordered so it packs after the first chunk.
	ctx := t.Context()
	s, failFrom, pushes, upper := heldTestController(t)
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	late := addSyncDirtyBlock(t, ctx, wtx, upper.StoreOps, "late block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s.updateDirtyState(ctx)
	lateKey := late.GetHash().MarshalString()
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.setHold(map[string]struct{}{lateKey: {}}, false)
	})
	s.corruptRecheck.Store(true)

	// The first push fails: the recheck survives and the hold stays.
	failFrom.Store(1)
	var held *heldError
	if err := s.FlushNowUnordered(ctx); err == nil || errors.As(err, &held) {
		t.Fatalf("failing flush = %v; want the push error", err)
	}
	var known map[string]struct{}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { known = s.hold.blocks })
	if !s.corruptRecheck.Load() || !hasKey(known, lateKey) {
		t.Fatalf("after a failed push: recheck %v, held %v; want both kept", s.corruptRecheck.Load(), hasKey(known, lateKey))
	}

	// The next flush reads the block, uploads it and clears the hold.
	failFrom.Store(0)
	pushes.Store(0)
	if err := s.FlushNowUnordered(ctx); err != nil {
		t.Fatalf("flush after the failure = %v; want the block read and uploaded", err)
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { known = s.hold.blocks })
	if len(known) != 0 || pushes.Load() == 0 {
		t.Fatalf("after the flush: %d held, %d pushes; want none held", len(known), pushes.Load())
	}
}

// TestSyncReclaimWalkFailsOnMissingOrCorrupt checks that the reclaim walk
// fails on a root it cannot read, missing everywhere or corrupt in the cache,
// rather than judging the blocks under it dead.
func TestSyncReclaimWalkFailsOnMissingOrCorrupt(t *testing.T) {
	s := newDirtySyncExecuteTestController(t, nil, nil)
	held, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := block.BuildBlockRef([]byte("absent block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.upper = &corruptUpperStore{StoreOps: s.upper, bad: held.GetHash()}
	if _, err := s.walkLive(t.Context(), []*block.BlockRef{absent}); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("walk over a missing root = %v; want not found", err)
	}
	if _, err := s.walkLive(t.Context(), []*block.BlockRef{held}); !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("walk over a corrupt root = %v; want corrupt", err)
	}
}
