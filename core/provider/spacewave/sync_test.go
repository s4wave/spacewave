package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_delta "github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestSyncPull_Success verifies SyncPull sends a GET to /sync/pull and returns the body.
func TestSyncPull_Success(t *testing.T) {
	resp := &packfile.PullResponse{
		Entries: []*packfile.PackfileEntry{
			{Id: "pack-001", BlockCount: 5},
			{Id: "pack-002", BlockCount: 3},
		},
	}
	respData, err := resp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

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

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	data, err := cli.SyncPull(context.Background(), "test-res", "")
	if err != nil {
		t.Fatalf("SyncPull: %v", err)
	}

	// Parse the returned data and verify it round-trips.
	parsed := &packfile.PullResponse{}
	if err := parsed.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal pull response: %v", err)
	}
	if len(parsed.GetEntries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(parsed.GetEntries()))
	}
	if parsed.GetEntries()[0].GetId() != "pack-001" {
		t.Fatalf("unexpected first entry ID: %s", parsed.GetEntries()[0].GetId())
	}
}

// TestSyncPull_WithSince verifies SyncPull adds a since query parameter.
func TestSyncPull_WithSince(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")
		if since != "pack-005" {
			t.Errorf("expected since=pack-005, got %q", since)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err := cli.SyncPull(context.Background(), "test-res", "pack-005")
	if err != nil {
		t.Fatalf("SyncPull with since: %v", err)
	}
}

// TestSyncPull_ServerError verifies SyncPull returns an error on server failure.
func TestSyncPull_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err := cli.SyncPull(context.Background(), "test-res", "")
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

// TestSyncPush_MissingWriteTicketExecutor verifies SyncPush fails locally when
// the write-ticket executor is unavailable.
func TestSyncPush_MissingWriteTicketExecutor(t *testing.T) {
	fileContent := []byte("packfile-binary-data-for-testing")
	h := sha256.Sum256(fileContent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

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

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	err = cli.SyncPush(context.Background(), "test-res", "test-pack-id", 42, tmpFile.Name(), h[:], nil, packfile.BloomFormatVersionV1)
	if err == nil || !strings.Contains(err.Error(), "missing write-ticket executor") {
		t.Fatalf("expected missing write-ticket executor error, got %v", err)
	}
}

// TestSyncPush_UsesWriteTicketWhenConfigured verifies SyncPush switches to the
// write-ticket proof path when the shared ticket executor is configured.
func TestSyncPush_UsesWriteTicketWhenConfigured(t *testing.T) {
	fileContent := []byte("packfile-binary-data-for-testing")
	h := sha256.Sum256(fileContent)
	bloomFilter := []byte("bloom-filter-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/bstore/test-res/sync/push" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Write-Ticket"); got != "ticket-push" {
			t.Errorf("unexpected write ticket: %q", got)
		}
		if r.Header.Get("X-Signature") != "" {
			t.Error("signed auth should not be used on the write-ticket path")
		}
		if r.Header.Get("X-Peer-ID") != "" {
			t.Error("write-ticket path should not set X-Peer-ID on the outer request")
		}
		if got := r.Header.Get("X-Sw-Hash"); got != hex.EncodeToString(h[:]) {
			t.Errorf("unexpected X-Sw-Hash: %q", got)
		}
		if got := r.Header.Get(SeedReasonHeader); got != string(SeedReasonMutation) {
			t.Errorf("unexpected seed reason: %q", got)
		}

		body, _ := io.ReadAll(r.Body)
		if string(body) != string(fileContent) {
			t.Errorf("body mismatch: got %d bytes, want %d", len(body), len(fileContent))
		}

		proofB64 := r.Header.Get("X-Write-Proof")
		if proofB64 == "" {
			t.Fatal("missing X-Write-Proof")
		}
		proofBytes, err := base64.StdEncoding.DecodeString(proofB64)
		if err != nil {
			t.Fatalf("decode proof: %v", err)
		}
		var proof api.WriteTicketProof
		if err := proof.UnmarshalVT(proofBytes); err != nil {
			t.Fatalf("unmarshal proof: %v", err)
		}
		var payload api.WriteTicketProofPayload
		if err := payload.UnmarshalVT(proof.GetPayload()); err != nil {
			t.Fatalf("unmarshal proof payload: %v", err)
		}
		if payload.GetTicket() != "ticket-push" {
			t.Errorf("unexpected proof ticket: %q", payload.GetTicket())
		}
		if payload.GetMethod() != http.MethodPost {
			t.Errorf("unexpected proof method: %q", payload.GetMethod())
		}
		if payload.GetPath() != "/api/bstore/test-res/sync/push" {
			t.Errorf("unexpected proof path: %q", payload.GetPath())
		}
		if payload.GetContentLength() != int64(len(body)) {
			t.Errorf("unexpected proof content length: %d", payload.GetContentLength())
		}
		if payload.GetBodyHashHex() != hex.EncodeToString(h[:]) {
			t.Errorf("unexpected proof body hash: %q", payload.GetBodyHashHex())
		}
		if payload.GetSignedHeaders() != "content-type=application/octet-stream,x-block-count=42,x-bloom-filter="+base64.StdEncoding.EncodeToString(bloomFilter)+",x-pack-id=test-pack-id" {
			t.Errorf("unexpected proof signed headers: %q", payload.GetSignedHeaders())
		}

		w.WriteHeader(http.StatusOK)
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
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "test-res" {
			t.Errorf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceBstoreSyncPush {
			t.Errorf("unexpected audience: %s", audience)
		}
		return fn("ticket-push")
	}

	err = cli.SyncPush(
		context.Background(),
		"test-res",
		"test-pack-id",
		42,
		tmpFile.Name(),
		h[:],
		bloomFilter,
		packfile.BloomFormatVersionV1,
	)
	if err != nil {
		t.Fatalf("SyncPush: %v", err)
	}
}

// TestSyncPushData_UsesWriteTicketWhenConfigured verifies SyncPushData also
// uses the write-ticket proof path when configured.
func TestSyncPushData_UsesWriteTicketWhenConfigured(t *testing.T) {
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)
	bloomFilter := []byte("bloom-filter-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Write-Ticket"); got != "ticket-push-data" {
			t.Errorf("unexpected write ticket: %q", got)
		}
		if r.Header.Get("X-Signature") != "" {
			t.Error("signed auth should not be used on the write-ticket path")
		}
		if got := r.Header.Get("X-Block-Count"); got != strconv.Itoa(3) {
			t.Errorf("unexpected X-Block-Count: %q", got)
		}
		if got := r.Header.Get("X-Bloom-Filter"); got != base64.StdEncoding.EncodeToString(bloomFilter) {
			t.Errorf("unexpected X-Bloom-Filter: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != string(packData) {
			t.Errorf("unexpected body: %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "test-res" {
			t.Errorf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceBstoreSyncPush {
			t.Errorf("unexpected audience: %s", audience)
		}
		return fn("ticket-push-data")
	}

	err := cli.SyncPushData(
		context.Background(),
		"test-res",
		"test-pack-id",
		3,
		packData,
		h[:],
		bloomFilter,
		packfile.BloomFormatVersionV1,
	)
	if err != nil {
		t.Fatalf("SyncPushData: %v", err)
	}
}

// TestSyncPushData_EnableDirectWriteTickets verifies a standalone session
// client can mint write tickets directly when no ProviderAccount owns it.
func TestSyncPushData_EnableDirectWriteTickets(t *testing.T) {
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)
	bloomFilter := []byte("bloom-filter-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/write-ticket/test-res/bstore-sync-push":
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			resp := &api.TicketResponse{Ticket: "ticket-direct"}
			data, err := resp.MarshalVT()
			if err != nil {
				t.Fatalf("marshal ticket response: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case "/api/bstore/test-res/sync/push":
			if got := r.Header.Get("X-Write-Ticket"); got != "ticket-direct" {
				t.Errorf("unexpected write ticket: %q", got)
			}
			if got := r.Header.Get("X-Bloom-Filter"); got != base64.StdEncoding.EncodeToString(bloomFilter) {
				t.Errorf("unexpected X-Bloom-Filter: %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != string(packData) {
				t.Errorf("unexpected body: %q", body)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.EnableDirectWriteTickets()

	err := cli.SyncPushData(
		context.Background(),
		"test-res",
		"test-pack-id",
		3,
		packData,
		h[:],
		bloomFilter,
		packfile.BloomFormatVersionV1,
	)
	if err != nil {
		t.Fatalf("SyncPushData: %v", err)
	}
}

// TestSyncPull_BlockedError verifies SyncPull returns an error classified as blocked.
func TestSyncPull_BlockedError(t *testing.T) {
	errResp := &api.ErrorResponse{
		Code:    "dmca_blocked",
		Message: "This resource has been disabled in response to a DMCA takedown notice.",
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(451)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err = cli.SyncPull(context.Background(), "test-res", "")
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
	errResp := &api.ErrorResponse{
		Code:      "dmca_blocked",
		Message:   "blocked",
		Retryable: true, // server says retryable, but permanentCodes overrides
	}
	respData, err := errResp.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(451)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	_, err = cli.SyncPull(context.Background(), "test-res", "")
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
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-push")
	}

	err = cli.SyncPush(context.Background(), "test-res", "test-pack-id", 1, tmpFile.Name(), h[:], bloomFilter, packfile.BloomFormatVersionV1)
	if err == nil {
		t.Fatal("expected error for server failure")
	}
}

func TestSyncPushData_MissingBloomFilter(t *testing.T) {
	packData := []byte("inline-pack-data")
	h := sha256.Sum256(packData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

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

// TestSyncPushData_MissingWriteTicketExecutor verifies SyncPushData fails
// locally when the write-ticket executor is unavailable.
func TestSyncPushData_MissingWriteTicketExecutor(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "missing write-ticket executor") {
		t.Fatalf("expected missing write-ticket executor error, got %v", err)
	}
}

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

func TestSyncControllerExecuteExitsOnCanceledThresholdContext(t *testing.T) {
	s := &syncController{
		conf: &SyncConfig{SizeThresholdBytes: 1},
	}
	s.dirtySize = 1

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

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
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-push")
	}

	s := newDirtySyncExecuteTestController(t, cli, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.Execute(ctx)
	}()

	select {
	case <-requests:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected initial sync push")
	}

	select {
	case <-requests:
		t.Fatal("retry-after was not honored by dirty sync")
	case <-time.After(1200 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

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
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-push")
	}

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
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected initial sync push")
	}

	select {
	case <-requests:
		t.Fatal("gated access denial retried without account state change")
	case <-time.After(1200 * time.Millisecond):
	}

	gate.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	select {
	case <-requests:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected account state change to wake gated dirty sync")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected Execute to stop when context is canceled")
	}
}

type syncFlushTestStore struct {
	block.NopStoreOps
	blocks map[string][]byte
}

type syncPackTransport struct {
	data []byte
}

type syncErrorTransport struct {
	err error
}

func (t *syncPackTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}
	end := min(off+int64(length), int64(len(t.data)))
	return bytes.Clone(t.data[off:end]), nil
}

func (t *syncErrorTransport) Fetch(context.Context, int64, int) ([]byte, error) {
	return nil, t.err
}

func (s *syncFlushTestStore) GetHashType() hash.HashType {
	return hash.RecommendedHashType
}

func (s *syncFlushTestStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, nil
}

func (s *syncFlushTestStore) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, ok := s.blocks[ref.GetHash().MarshalString()]
	return data, ok, nil
}

func (s *syncFlushTestStore) GetBlockExists(_ context.Context, ref *block.BlockRef) (bool, error) {
	_, ok := s.blocks[ref.GetHash().MarshalString()]
	return ok, nil
}

func (s *syncFlushTestStore) RmBlock(context.Context, *block.BlockRef) error {
	return nil
}

func (s *syncFlushTestStore) StatBlock(_ context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, ok := s.blocks[ref.GetHash().MarshalString()]
	if !ok {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

// _ is a type assertion
var _ block.StoreOps = ((*syncFlushTestStore)(nil))

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
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		item := items[idx]
		idx++
		return item.h, item.data, nil
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
				hash.RecommendedHashType,
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

func newSyncTestErrorLowerPackfileStore() *packfile_store.PackfileStore {
	lower := packfile_store.NewPackfileStore(
		func(packID string, size int64) (*packfile_store.PackReader, error) {
			return packfile_store.NewPackReader(
				packID,
				size,
				&syncErrorTransport{err: io.ErrUnexpectedEOF},
				hash.RecommendedHashType,
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

type syncTestRefGraph struct {
	out map[string][]string
	in  map[string][]string
}

func newSyncTestRefGraph() *syncTestRefGraph {
	return &syncTestRefGraph{
		out: make(map[string][]string),
		in:  make(map[string][]string),
	}
}

func (g *syncTestRefGraph) add(subject, object string) {
	g.out[subject] = append(g.out[subject], object)
	g.in[object] = append(g.in[object], subject)
}

func (g *syncTestRefGraph) GetOutgoingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.out[node]), nil
}

func (g *syncTestRefGraph) GetIncomingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.in[node]), nil
}

func newSyncTestKvStore() kvtx.Store {
	return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
}

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
		keys = append(keys, string(entry.GetKey()))
	}
	return keys
}

func addSyncDirtyBlock(
	t *testing.T,
	ctx context.Context,
	wtx kvtx.Tx,
	upper *syncFlushTestStore,
	body string,
) *block.BlockRef {
	t.Helper()
	data := []byte(body)
	h, err := hash.Sum(hash.RecommendedHashType, data)
	if err != nil {
		t.Fatalf("sum block hash: %v", err)
	}
	upper.blocks[h.MarshalString()] = data
	if err := wtx.Set(
		ctx,
		[]byte("dirty/"+h.MarshalString()),
		[]byte(strconv.Itoa(len(data))),
	); err != nil {
		t.Fatalf("set dirty key: %v", err)
	}
	return block.NewBlockRef(h)
}

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

	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 1)}
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
		conf:       &SyncConfig{SizeThresholdBytes: 1, InactivityTimeoutSecs: 1},
		gateBcast:  gate,
	}
	s.recalcDirtySize(ctx)
	return s
}

func TestSyncControllerFlushChunksLargeDirtySet(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	pushSizes := make([]int, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bstore/test-res/sync/push" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		pushSizes = append(pushSizes, len(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "test-res" {
			t.Fatalf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceBstoreSyncPush {
			t.Fatalf("unexpected audience: %s", audience)
		}
		return fn("ticket-push")
	}

	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 3)}
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	for i := range 3 {
		data := bytes.Repeat([]byte{byte(i + 1)}, 24*1024*1024)
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatalf("sum block hash: %v", err)
		}
		upper.blocks[h.MarshalString()] = data
		if err := wtx.Set(
			ctx,
			[]byte("dirty/"+h.MarshalString()),
			[]byte(strconv.Itoa(len(data))),
		); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}
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
	}

	if err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if len(pushSizes) != 2 {
		t.Fatalf("expected 2 sync pushes, got %d", len(pushSizes))
	}
	for i, size := range pushSizes {
		if int64(size) > packfile_delta.DefaultMaxChunkBytes {
			t.Fatalf("push %d exceeded chunk limit: %d", i, size)
		}
	}
	if len(mfst.GetEntries()) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(mfst.GetEntries()))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())

	rtx, err := dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	dirtyCount := 0
	err = rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		dirtyCount++
		return nil
	})
	if err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	if dirtyCount != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", dirtyCount)
	}
}

func TestSyncControllerFlushDedupesLowerBlocks(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	var pushedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bstore/test-res/sync/push" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		pushedBody = bytes.Clone(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "test-res" {
			t.Fatalf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceBstoreSyncPush {
			t.Fatalf("unexpected audience: %s", audience)
		}
		return fn("ticket-push")
	}

	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 2)}
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	duplicate := addSyncDirtyBlock(t, ctx, wtx, upper, "duplicate dirty block")
	fresh := addSyncDirtyBlock(t, ctx, wtx, upper, "fresh dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

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

	if len(mfst.GetEntries()) != 1 {
		t.Fatalf("expected 1 manifest entry, got %d", len(mfst.GetEntries()))
	}
	want := []string{fresh.GetHash().MarshalString()}
	if got := readPackPhysicalKeys(t, pushedBody); !slices.Equal(got, want) {
		t.Fatalf("physical pack keys = %v, want %v; duplicate=%s", got, want, duplicate.GetHash().MarshalString())
	}

	rtx, err := dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	dirtyCount := 0
	err = rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		dirtyCount++
		return nil
	})
	if err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	if dirtyCount != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", dirtyCount)
	}
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

func TestSyncControllerFlushAllDuplicateDirtyBlocksSkipsPush(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	var pushCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

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

	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 2)}
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
	if pushCount != 0 {
		t.Fatalf("expected no sync pushes, got %d", pushCount)
	}
	if len(mfst.GetEntries()) != 0 {
		t.Fatalf("expected no manifest entries, got %d", len(mfst.GetEntries()))
	}

	rtx, err := dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	dirtyCount := 0
	err = rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		dirtyCount++
		return nil
	})
	if err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	if dirtyCount != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", dirtyCount)
	}
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

func TestSyncControllerFlushDuplicateProbeErrorPreservesDirty(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, "https://example.invalid", DefaultSigningEnvPrefix, priv, pid.String())
	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 1)}
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
		lower:      newSyncTestErrorLowerPackfileStore(),
		upper:      upper,
	}

	if err := s.flush(ctx, true); err == nil {
		t.Fatal("expected duplicate probe error")
	}

	rtx, err := dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	defer rtx.Discard()
	dirtyCount := 0
	err = rtx.ScanPrefix(ctx, []byte("dirty/"), func(_, _ []byte) error {
		dirtyCount++
		return nil
	})
	if err != nil {
		t.Fatalf("scan dirty keys: %v", err)
	}
	if dirtyCount != 1 {
		t.Fatalf("expected dirty key to remain after probe error, got %d", dirtyCount)
	}
}

func TestSyncControllerFlushOrdersBlocksByGCGraph(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	var pushedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bstore/test-res/sync/push" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		pushedBody = bytes.Clone(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

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

	upper := &syncFlushTestStore{blocks: make(map[string][]byte, 4)}
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	stray := addSyncDirtyBlock(t, ctx, wtx, upper, "stray")
	childA := addSyncDirtyBlock(t, ctx, wtx, upper, "child-a")
	rootB := addSyncDirtyBlock(t, ctx, wtx, upper, "root-b")
	rootA := addSyncDirtyBlock(t, ctx, wtx, upper, "root-a")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	graph := newSyncTestRefGraph()
	graph.add(block_gc.ObjectIRI("object-b"), block_gc.BlockIRI(rootB))
	graph.add(block_gc.ObjectIRI("object-a"), block_gc.BlockIRI(rootA))
	graph.add(block_gc.BlockIRI(rootA), block_gc.BlockIRI(childA))

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

	want := []string{
		rootA.GetHash().MarshalString(),
		childA.GetHash().MarshalString(),
		rootB.GetHash().MarshalString(),
		stray.GetHash().MarshalString(),
	}
	if got := readPackPhysicalKeys(t, pushedBody); !slices.Equal(got, want) {
		t.Fatalf("physical pack order = %v, want %v", got, want)
	}
}

func TestSyncControllerFlushChunksBlockCountCeiling(t *testing.T) {
	ctx := context.Background()

	dirtyStore := newSyncTestKvStore()
	manifestStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, manifestStore)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	var pushBlockCounts []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bstore/test-res/sync/push" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		count, err := strconv.Atoi(r.Header.Get("X-Block-Count"))
		if err != nil {
			t.Fatalf("parse X-Block-Count: %v", err)
		}
		if r.Header.Get("X-Bloom-Filter") == "" {
			t.Fatal("missing X-Bloom-Filter")
		}
		pushBlockCounts = append(pushBlockCounts, count)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if resourceID != "test-res" {
			t.Fatalf("unexpected resource id: %s", resourceID)
		}
		if audience != writeTicketAudienceBstoreSyncPush {
			t.Fatalf("unexpected audience: %s", audience)
		}
		return fn("ticket-push")
	}

	blockCount := int(writer.DefaultMaxBlocksPerPack) + 1
	upper := &syncFlushTestStore{blocks: make(map[string][]byte, blockCount)}
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	for i := range blockCount {
		data := []byte("small dirty block " + strconv.Itoa(i))
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatalf("sum block hash: %v", err)
		}
		upper.blocks[h.MarshalString()] = data
		if err := wtx.Set(
			ctx,
			[]byte("dirty/"+h.MarshalString()),
			[]byte(strconv.Itoa(len(data))),
		); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}
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
	}

	if err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if len(pushBlockCounts) != 2 {
		t.Fatalf("expected 2 sync pushes, got %d", len(pushBlockCounts))
	}
	total := 0
	for i, count := range pushBlockCounts {
		if count > int(writer.DefaultMaxBlocksPerPack) {
			t.Fatalf("push %d exceeded block ceiling: %d", i, count)
		}
		total += count
	}
	if total != blockCount {
		t.Fatalf("pushed block total = %d, want %d", total, blockCount)
	}
	if len(mfst.GetEntries()) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(mfst.GetEntries()))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())
}
