package provider_spacewave

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"

	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
)

type wrapperForwardTestStore struct {
	block.NopStoreOps
	id                string
	putBlockBatchHits int
	backgroundHits    int
	existsBatchHits   int
}

func TestBstoreTrackerDetectsPublicReadSpaceBlockStore(t *testing.T) {
	acc := &ProviderAccount{}
	acc.SetSharedObjectMetadata("space-1", &api.SpaceMetadataResponse{
		ObjectType:  "space",
		PublicRead:  true,
		DisplayName: "Public Space",
	})

	tracker := &bstoreTracker{a: acc, id: "space-1"}
	if !tracker.isPublicReadSpaceBlockStore(context.Background()) {
		t.Fatal("expected public_read Space with matching block store id")
	}
}

func TestBstoreTrackerRejectsNonPublicReadBlockStore(t *testing.T) {
	acc := &ProviderAccount{}
	acc.SetSharedObjectMetadata("space-1", &api.SpaceMetadataResponse{
		ObjectType:  "space",
		PublicRead:  false,
		DisplayName: "Private Space",
	})

	tracker := &bstoreTracker{a: acc, id: "space-1"}
	if tracker.isPublicReadSpaceBlockStore(context.Background()) {
		t.Fatal("private Space should use the authenticated Worker read path")
	}
}

func TestPublicReadRemoteRefreshUsesAnonymousCdnManifest(t *testing.T) {
	ptr := &alpha_cdn.CdnRootPointer{
		SpaceId: "space-1",
		Packs: []*packfile.PackfileEntry{{
			Id:         "pack-1",
			BlockCount: 1,
			SizeBytes:  128,
		}},
	}
	raw, err := ptr.MarshalVT()
	if err != nil {
		t.Fatalf("marshal pointer: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space-1/root.packedmsg" {
			t.Fatalf("unexpected anonymous CDN request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(packedmsg.EncodePackedMessage(raw)))
	}))
	defer srv.Close()

	remote := newPublicReadRemote(srv.Client(), srv.URL, "space-1", nil)
	if err := remote.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	entries := remote.Entries()
	if len(entries) != 1 || entries[0].GetId() != "pack-1" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if got := remote.lower.SnapshotStats().ManifestEntries; got != 1 {
		t.Fatalf("lower manifest entries = %d, want 1", got)
	}
}

func (s *wrapperForwardTestStore) GetID() string              { return s.id }
func (s *wrapperForwardTestStore) GetHashType() hash.HashType { return 0 }
func (s *wrapperForwardTestStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, nil
}

func (s *wrapperForwardTestStore) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *wrapperForwardTestStore) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *wrapperForwardTestStore) StatBlock(_ context.Context, _ *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}
func (s *wrapperForwardTestStore) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }
func (s *wrapperForwardTestStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return nil
}

func (s *wrapperForwardTestStore) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundHits++
	return nil, false, nil
}

func (s *wrapperForwardTestStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return make([]bool, len(refs)), nil
}

var (
	_ block_store.Store = ((*wrapperForwardTestStore)(nil))
	_ block.StoreOps    = ((*wrapperForwardTestStore)(nil))
)

func TestBlockStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := &wrapperForwardTestStore{id: "test"}
	store := &BlockStore{store: inner}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: &block.BlockRef{}}}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}
	if inner.putBlockBatchHits != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBlockBatchHits)
	}

	if _, _, err := store.PutBlockBackground(ctx, []byte("hello"), nil); err != nil {
		t.Fatalf("PutBlockBackground failed: %v", err)
	}
	if inner.backgroundHits != 1 {
		t.Fatalf("expected 1 PutBlockBackground call, got %d", inner.backgroundHits)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{{}}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}

func TestBlockStoreForceSyncDetachesCancellation(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	var sawDeadline bool
	store := &BlockStore{
		forceSync: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				t.Fatalf("force sync context should not inherit cancellation: %v", err)
			}
			_, sawDeadline = ctx.Deadline()
			return nil
		},
	}

	if err := store.ForceSync(parentCtx); err != nil {
		t.Fatalf("ForceSync returned error: %v", err)
	}
	if !sawDeadline {
		t.Fatal("expected force sync context to have a timeout deadline")
	}

	start := time.Now()
	if err := store.ForceSync(context.Background()); err != nil {
		t.Fatalf("ForceSync with live context returned error: %v", err)
	}
	if time.Since(start) >= forceSyncTimeout {
		t.Fatal("ForceSync should not wait for the timeout")
	}
}

type dirtyBatchTestStore struct {
	block.NopStoreOps
	putBlockBatchHits int
	existsBatchHits   int
	exists            []bool
}

func (s *dirtyBatchTestStore) GetHashType() hash.HashType { return 0 }
func (s *dirtyBatchTestStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, nil
}

func (s *dirtyBatchTestStore) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *dirtyBatchTestStore) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *dirtyBatchTestStore) StatBlock(_ context.Context, _ *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}
func (s *dirtyBatchTestStore) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }
func (s *dirtyBatchTestStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return nil
}

func (s *dirtyBatchTestStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	if s.exists != nil {
		return slices.Clone(s.exists), nil
	}
	return make([]bool, len(refs)), nil
}

// _ is a type assertion
var _ block.StoreOps = ((*dirtyBatchTestStore)(nil))

func TestDirtyTrackingStoreForwardsBatch(t *testing.T) {
	ctx := context.Background()
	inner := &dirtyBatchTestStore{}
	var dirtyMarks int
	store := &dirtyTrackingStore{
		store: inner,
		markDirty: func(_ context.Context, _ *hash.Hash, _ int64) {
			dirtyMarks++
		},
	}

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: ref1, Data: []byte("hello")},
		{Ref: ref2, Data: []byte("world")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}
	if inner.putBlockBatchHits != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBlockBatchHits)
	}
	if dirtyMarks != 2 {
		t.Fatalf("expected 2 dirty marks, got %d", dirtyMarks)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 advisory GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{{}}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 2 {
		t.Fatalf("expected 2 GetBlockExistsBatch calls, got %d", inner.existsBatchHits)
	}
}

func TestDirtyTrackingStoreBatchSkipsExistingBlocks(t *testing.T) {
	ctx := context.Background()
	inner := &dirtyBatchTestStore{exists: []bool{true, false}}
	var dirty []string
	store := &dirtyTrackingStore{
		store: inner,
		markDirty: func(_ context.Context, h *hash.Hash, _ int64) {
			dirty = append(dirty, h.MarshalString())
		},
	}

	existing, err := block.BuildBlockRef([]byte("existing"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef existing failed: %v", err)
	}
	fresh, err := block.BuildBlockRef([]byte("fresh"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef fresh failed: %v", err)
	}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: existing, Data: []byte("existing")},
		{Ref: fresh, Data: []byte("fresh")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}
	if inner.putBlockBatchHits != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBlockBatchHits)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
	want := []string{fresh.GetHash().MarshalString()}
	if !slices.Equal(dirty, want) {
		t.Fatalf("dirty marks = %v, want %v", dirty, want)
	}
}

func TestHTTPReaderAtReadsOffsetFromFullBodyFallback(t *testing.T) {
	data := []byte("0123456789abcdef")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if got := r.Header.Get("Range"); got != "bytes=0-15" {
			t.Fatalf("expected Range header bytes=0-15, got %q", got)
		}
		w.Header().Set("Content-Length", "16")
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	shared := packfile_store.NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		16,
		httpReaderPageSize,
		nil,
		nil,
	)
	rd := shared.ReaderAt(context.Background())

	buf := make([]byte, 4)
	n, err := rd.ReadAt(buf, 4)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt returned error: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes, got %d", n)
	}
	if got := string(buf); got != "4567" {
		t.Fatalf("expected 4567, got %q", got)
	}
}

func TestHTTPReaderAtReadAheadCache(t *testing.T) {
	data := []byte("0123456789abcdef")
	var reqs int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		if got := r.Header.Get("Range"); got != "bytes=0-15" {
			t.Fatalf("expected Range header bytes=0-15, got %q", got)
		}
		w.Header().Set("Content-Length", "16")
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	shared := packfile_store.NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		16,
		httpReaderPageSize,
		nil,
		nil,
	)
	rd := shared.ReaderAt(context.Background())

	buf := make([]byte, 4)
	n, err := rd.ReadAt(buf, 4)
	if err != nil && err != io.EOF {
		t.Fatalf("first ReadAt returned error: %v", err)
	}
	if n != 4 || string(buf) != "4567" {
		t.Fatalf("unexpected first read: n=%d data=%q", n, string(buf))
	}

	n, err = rd.ReadAt(buf, 8)
	if err != nil && err != io.EOF {
		t.Fatalf("second ReadAt returned error: %v", err)
	}
	if n != 4 || string(buf) != "89ab" {
		t.Fatalf("unexpected second read: n=%d data=%q", n, string(buf))
	}
	if reqs != 1 {
		t.Fatalf("expected 1 HTTP request, got %d", reqs)
	}
}

func TestHTTPReaderAtReusesPackReadTicket(t *testing.T) {
	data := []byte("0123456789abcdef")
	priv, pid := generateTestKeypair(t)
	sessionCli := NewSessionClient(
		http.DefaultClient,
		"https://spacewave.test",
		DefaultSigningEnvPrefix,
		priv,
		pid.String(),
	)
	resourceID := "01kny7hn4wp25f7t86xzww6bd6"
	reqs := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		start, end, ok := parseHTTPRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Fatalf("missing or invalid Range header: %q", r.Header.Get("Range"))
		}
		if reqs == 1 {
			if r.Header.Get(packReadTicketHeader) != "" {
				t.Fatalf("unexpected pack read ticket on first request")
			}
			if r.Header.Get("X-Signature") == "" {
				t.Fatalf("expected signed first request")
			}
			w.Header().Set(packReadTicketHeader, "ticket-1")
		} else {
			if got := r.Header.Get(packReadTicketHeader); got != "ticket-1" {
				t.Fatalf("expected pack read ticket on second request, got %q", got)
			}
			if r.Header.Get("X-Signature") != "" {
				t.Fatalf("expected second request to skip request signature")
			}
			if r.Header.Get("X-Peer-ID") != "" {
				t.Fatalf("expected second request to skip peer header")
			}
		}
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:end]); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	shared := packfile_store.NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		4,
		httpReaderPageSize,
		func(req *http.Request) error {
			return sessionCli.signPackReadRequest(req, resourceID)
		},
		func(resp *http.Response) {
			sessionCli.observePackReadResponse(resourceID, resp)
		},
	)
	rd := shared.ReaderAt(context.Background())

	buf := make([]byte, 4)
	n, err := rd.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("first ReadAt returned error: %v", err)
	}
	if n != 4 || string(buf) != "0123" {
		t.Fatalf("unexpected first read: n=%d data=%q", n, string(buf))
	}

	n, err = rd.ReadAt(buf, 8)
	if err != nil && err != io.EOF {
		t.Fatalf("second ReadAt returned error: %v", err)
	}
	if n != 4 || string(buf) != "89ab" {
		t.Fatalf("unexpected second read: n=%d data=%q", n, string(buf))
	}
	if reqs != 2 {
		t.Fatalf("expected 2 HTTP requests, got %d", reqs)
	}
}

func TestHTTPReaderAtRetainsMultipleRanges(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 8192)
	var reqs int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		start, end, ok := parseHTTPRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Fatalf("missing or invalid Range header: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:end]); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	shared := packfile_store.NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		16,
		httpReaderPageSize,
		nil,
		nil,
	)
	rd := shared.ReaderAt(context.Background())

	buf := make([]byte, 4)
	for _, off := range []int64{0, 70000, 0} {
		n, err := rd.ReadAt(buf, off)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d) returned error: %v", off, err)
		}
		if n != 4 {
			t.Fatalf("expected 4 bytes from offset %d, got %d", off, n)
		}
	}
	if reqs != 2 {
		t.Fatalf("expected 2 HTTP requests for two distinct cached ranges, got %d", reqs)
	}
}

func parseHTTPRangeHeader(h string, size int64) (start, end int64, ok bool) {
	var reqStart, reqEnd int64
	if _, err := fmt.Sscanf(h, "bytes=%d-%d", &reqStart, &reqEnd); err != nil {
		return 0, 0, false
	}
	if reqStart < 0 || reqEnd < reqStart || reqStart >= size {
		return 0, 0, false
	}
	if reqEnd >= size {
		reqEnd = size - 1
	}
	return reqStart, reqEnd + 1, true
}

type lowerReadTestStore struct {
	block.NopStoreOps
	ref  *block.BlockRef
	data []byte
}

func (s *lowerReadTestStore) GetHashType() hash.HashType { return hash.HashType_HashType_SHA256 }
func (s *lowerReadTestStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, nil
}

func (s *lowerReadTestStore) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.ref != nil && s.ref.EqualsRef(ref) {
		return bytes.Clone(s.data), true, nil
	}
	return nil, false, nil
}

func (s *lowerReadTestStore) GetBlockExists(_ context.Context, ref *block.BlockRef) (bool, error) {
	return s.ref != nil && s.ref.EqualsRef(ref), nil
}

func (s *lowerReadTestStore) StatBlock(_ context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if s.ref != nil && s.ref.EqualsRef(ref) {
		return &block.BlockStat{Ref: ref, Size: int64(len(s.data))}, nil
	}
	return nil, nil
}
func (s *lowerReadTestStore) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }

type upperPutRecorder struct {
	block.NopStoreOps
	puts int
}

func (s *upperPutRecorder) GetHashType() hash.HashType { return hash.HashType_HashType_SHA256 }
func (s *upperPutRecorder) PutBlock(_ context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.puts++
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

func (s *upperPutRecorder) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *upperPutRecorder) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *upperPutRecorder) StatBlock(_ context.Context, _ *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}
func (s *upperPutRecorder) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }

func TestNewCloudOverlayDoesNotDirtyLowerReads(t *testing.T) {
	data := []byte("alpha")
	ref, err := block.BuildBlockRef(data, &block.PutOpts{
		HashType: hash.HashType_HashType_SHA256,
	})
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	lower := &lowerReadTestStore{ref: ref, data: data}
	upper := &upperPutRecorder{}
	var dirtyMarks int
	dirtyUpper := &dirtyTrackingStore{
		store: upper,
		markDirty: func(context.Context, *hash.Hash, int64) {
			dirtyMarks++
		},
	}

	overlay := newCloudOverlay(context.Background(), lower, dirtyUpper)
	got, found, err := overlay.GetBlock(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetBlock returned error: %v", err)
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("expected lower read hit, got found=%v data=%q", found, string(got))
	}
	if upper.puts != 0 {
		t.Fatalf("expected no upper writeback on lower read, got %d puts", upper.puts)
	}
	if dirtyMarks != 0 {
		t.Fatalf("expected no dirty marks from lower read, got %d", dirtyMarks)
	}
}
