package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/packfile"
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
