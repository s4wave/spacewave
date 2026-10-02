package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	packfile_delta "github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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

// TestSyncPull_ServerError verifies SyncPull returns an error on server failure.
func TestSyncPull_ServerError(t *testing.T) {
	// Fail every request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	// The pull returns the failure.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	_, err := cli.SyncPull(context.Background(), "test-res", 0)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

// TestSyncPush verifies SyncPush admits the pack, uploads the file to the
// signed URL, and commits it.
func TestSyncPush(t *testing.T) {
	// Serve the push protocol for a pack of known bytes.
	fileContent := []byte("packfile-binary-data-for-testing")
	h := sha256.Sum256(fileContent)
	bloomFilter := []byte("bloom-filter-bytes")
	push := startTestPushServer(t)

	// Write the test file.
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-pack-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write(fileContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	// Push the file.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())
	err = cli.SyncPush(context.Background(), "test-res", "test-pack-id", 42, tmpFile.Name(), h[:], bloomFilter, packfile.BloomFormatVersionV1)
	if err != nil {
		t.Fatalf("SyncPush: %v", err)
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
		SizeBytes:          uint64(len(fileContent)),
		Sha256:             h[:],
	}
	if !packs[0].req.EqualVT(want) {
		t.Fatalf("push request = %v, want %v", packs[0].req, want)
	}
	if !bytes.Equal(packs[0].data, fileContent) {
		t.Fatalf("uploaded %q, want %q", packs[0].data, fileContent)
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

// TestSyncPull_BlockedError verifies SyncPull returns an error classified as blocked.
func TestSyncPull_BlockedError(t *testing.T) {
	// Encode a DMCA block error.
	errResp := &api.ErrorResponse{
		Code:    "dmca_blocked",
		Message: "This resource has been disabled in response to a DMCA takedown notice.",
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Serve the error with status 451.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(451)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// The pull fails as blocked and nothing else.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	_, err = cli.SyncPull(context.Background(), "test-res", 0)
	if err == nil {
		t.Fatal("expected error for 451 status")
	}
	if !isBlockedCloudError(err) {
		t.Fatalf("expected blocked cloud error, got: %v", err)
	}
	if isUnauthCloudError(err) {
		t.Fatal("should not be classified as unauth")
	}
	if isIdleableCloudError(err) {
		t.Fatal("should not be classified as idleable")
	}
}

// TestSyncPull_BlockedError_NotRetryable verifies dmca_blocked errors are not retryable.
func TestSyncPull_BlockedError_NotRetryable(t *testing.T) {
	// Encode a DMCA block error.
	errResp := &api.ErrorResponse{
		Code:      "dmca_blocked",
		Message:   "blocked",
		Retryable: true, // server says retryable, but permanentCodes overrides
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	// Serve the error with status 451.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(451)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	// The pull fails as not retryable.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	_, err = cli.SyncPull(context.Background(), "test-res", 0)
	if err == nil {
		t.Fatal("expected error for 451 status")
	}
	if !isNonRetryableCloudError(err) {
		t.Fatalf("expected non-retryable cloud error, got: %v", err)
	}
}

// TestSyncPush_MissingFile verifies SyncPush returns an error for missing file.
func TestSyncPush_MissingFile(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, "http://localhost", DefaultSigningEnvPrefix, priv, pid.String())

	err := cli.SyncPush(context.Background(), "test-res", "test-pack-id", 1, "/nonexistent/file.bin", []byte("hash"), nil, packfile.BloomFormatVersionV1)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestSyncPush_ServerError verifies SyncPush returns error on server failure.
func TestSyncPush_ServerError(t *testing.T) {
	fileContent := []byte("test-data")
	h := sha256.Sum256(fileContent)
	bloomFilter := []byte("bloom-filter-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	tmpFile, err := os.CreateTemp(t.TempDir(), "test-pack-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write(fileContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err = cli.SyncPush(context.Background(), "test-res", "test-pack-id", 1, tmpFile.Name(), h[:], bloomFilter, packfile.BloomFormatVersionV1)
	if err == nil {
		t.Fatal("expected error for server failure")
	}
}

// TestSyncPushData_MissingBloomFilter rejects uploads without a bloom filter.
func TestSyncPushData_MissingBloomFilter(t *testing.T) {
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

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
	s := &syncController{}

	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := s.pushPackfile(parentCtx, "test-pack-id", 7, func(
		ctx context.Context,
		packID string,
		blockCount int,
	) error {
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
	ctx := context.Background()
	pullRequests := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/pull") {
			pullRequests <- struct{}{}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

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
	lower := packfile_store.NewPackfileStore(nil, nil)
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
		lower:      packfile_store.NewPackfileStore(nil, nil),
	}

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
	ctx := t.Context()
	pushing := make(chan struct{})
	pulled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/pull") {
			close(pulled)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(pushing)
		select {
		case <-pulled:
		case <-time.After(10 * time.Second):
			t.Error("the pull waited for the push")
		}
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      newSyncTestKvStore(),
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil),
		upper:      newSyncTestBlockStore(),
	}
	data := []byte("pull during push")
	ref, _, err := s.upper.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatalf("put block: %v", err)
	}
	if err := s.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}

	flushed := make(chan error, 1)
	go func() { flushed <- s.FlushNow(ctx) }()
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
	ctx := context.Background()
	errResp := &api.ErrorResponse{
		Code:    "rbac_denied",
		Message: "access denied",
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	pullRequests := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sync/pull") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		pullRequests <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

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
		lower:      packfile_store.NewPackfileStore(nil, nil),
	}

	err = s.Init(ctx)
	if !isCloudAccessGatedError(err) {
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
	errResp := &api.ErrorResponse{
		Code:      "rate_limited",
		Message:   "retry later",
		Retryable: true,
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	s := newDirtySyncExecuteTestController(t, cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected initial sync push")
	}

	select {
	case <-requests:
		t.Fatal("retry-after was not honored by dirty sync")
	case <-time.After(syncExecuteNoRetryWindow):
	}

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
	errResp := &api.ErrorResponse{
		Code:      "account_read_only",
		Message:   "Account is in a read-only lifecycle state",
		Retryable: false,
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	requests := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

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

	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected initial sync push")
	}

	select {
	case <-requests:
		t.Fatal("gated access denial retried without account state change")
	case <-time.After(syncExecuteNoRetryWindow):
	}

	gate.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	select {
	case <-requests:
	case <-time.After(syncExecuteRequestTimeout):
		t.Fatal("expected account state change to wake gated dirty sync")
	}

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
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-push")
	}
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
		stored.RefsKnown = true
	}
	return stored, err
}

// newSyncTestBlockStore constructs the provider block test store.
func newSyncTestBlockStore() block.StoreOps {
	return syncTestBlockStore{StoreOps: newProviderSpacewaveTestBlockStore(hash.RecommendedHashType)}
}

// newSyncTestLowerPackfileStore packs fixture blocks into a readable lower store.
func newSyncTestLowerPackfileStore(t *testing.T, blocks map[string][]byte) *packfile_store.PackfileStore {
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

	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		item := items[idx]
		idx++
		return item.h, &block.StoredBlock{Data: item.data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatalf("pack lower blocks: %v", err)
	}

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
	)
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
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncErrorTransport{err: io.ErrUnexpectedEOF},
			), nil
		},
		nil,
	)
	lower.UpdateManifest([]*packfile.PackfileEntry{{
		Id:                 "error-pack",
		BloomFilter:        []byte("invalid-bloom"),
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         1,
		SizeBytes:          1,
	}})
	return lower
}

// syncTestRefGraph stores directed block references.
type syncTestRefGraph struct {
	out map[string][]string
	in  map[string][]string
}

// newSyncTestRefGraph constructs an empty reference graph.
func newSyncTestRefGraph() *syncTestRefGraph {
	return &syncTestRefGraph{
		out: make(map[string][]string),
		in:  make(map[string][]string),
	}
}

// add inserts a directed reference.
func (g *syncTestRefGraph) add(subject, object string) {
	g.out[subject] = append(g.out[subject], object)
	g.in[object] = append(g.in[object], subject)
}

// GetOutgoingRefs returns references originating at the node.
func (g *syncTestRefGraph) GetOutgoingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.out[node]), nil
}

// GetIncomingRefs returns references targeting the node.
func (g *syncTestRefGraph) GetIncomingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.in[node]), nil
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
	t.Helper()
	reader, err := kvfile.BuildReader(bytes.NewReader(body), uint64(len(body)))
	if err != nil {
		t.Fatalf("build pack reader: %v", err)
	}
	var entries []*kvfile.IndexEntry
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		entries = append(entries, ie.CloneVT())
		return nil
	})
	if err != nil {
		t.Fatalf("scan pack entries: %v", err)
	}
	slices.SortFunc(entries, func(a, b *kvfile.IndexEntry) int {
		if a.GetOffset() < b.GetOffset() {
			return -1
		}
		if a.GetOffset() > b.GetOffset() {
			return 1
		}
		return 0
	})
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
	t.Helper()
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

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

	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil),
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
		lower:      packfile_store.NewPackfileStore(nil, nil),
		upper:      upper,
	}
	if err := s.flush(ctx, true); err != nil {
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
		lower:      packfile_store.NewPackfileStore(nil, nil),
		upper:      upper,
	}
	if err := s.flush(ctx, false); err == nil {
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
	if n, want := countSyncDirtyKeys(t, ctx, dirtyStore), blockCount-pushedBlocks; n != want {
		t.Fatalf("dirty entries = %d, want %d unpushed blocks", n, want)
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
	s.telemetry = telemetry
	if err := s.flush(ctx, true); err != nil {
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
	s.telemetry = telemetry
	if err := s.flush(ctx, true); err != nil {
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
	if err := s.flush(ctx, true); err == nil {
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

	// Root each object at one block, with child-a under root-a.
	graph := newSyncTestRefGraph()
	graph.add(block_gc.ObjectIRI("object-b"), block_gc.BlockIRI(rootB))
	graph.add(block_gc.ObjectIRI("object-a"), block_gc.BlockIRI(rootA))
	graph.add(block_gc.BlockIRI(rootA), block_gc.BlockIRI(childA))

	// Flush in graph order.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      packfile_store.NewPackfileStore(nil, nil),
		upper:      upper,
		refGraph:   graph,
	}
	if err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Each root precedes its children, then the unreferenced block.
	want := []string{
		rootA.GetHash().MarshalString(),
		childA.GetHash().MarshalString(),
		rootB.GetHash().MarshalString(),
		stray.GetHash().MarshalString(),
	}
	if got := readPackPhysicalKeys(t, testPushedBody(t, push)); !slices.Equal(got, want) {
		t.Fatalf("physical pack order = %v, want %v", got, want)
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
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
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
		lower:      packfile_store.NewPackfileStore(nil, nil),
		upper:      upper,
	}
	if err := s.flush(ctx, true); err != nil {
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
