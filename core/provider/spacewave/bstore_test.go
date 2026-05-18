package provider_spacewave

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"

	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
)

type wrapperForwardTestStore struct {
	block.StoreOps
	id                string
	putBlockHits      int
	putBlockBatchHits int
	backgroundHits    int
	existsBatchHits   int
}

func newProviderSpacewaveTestBlockStore(hashType hash.HashType) block.StoreOps {
	return block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hashType,
		false,
	)
}

func newWrapperForwardTestStore(id string, hashType hash.HashType) *wrapperForwardTestStore {
	return &wrapperForwardTestStore{
		StoreOps: newProviderSpacewaveTestBlockStore(hashType),
		id:       id,
	}
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

func (s *wrapperForwardTestStore) GetID() string { return s.id }

func (s *wrapperForwardTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putBlockHits++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *wrapperForwardTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *wrapperForwardTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundHits++
	return s.StoreOps.PutBlockBackground(ctx, data, opts)
}

func (s *wrapperForwardTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

var (
	_ block_store.Store = ((*wrapperForwardTestStore)(nil))
	_ block.StoreOps    = ((*wrapperForwardTestStore)(nil))
)

func TestBlockStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperForwardTestStore("test", 0)
	store := &BlockStore{store: inner}
	ref, err := block.BuildBlockRef([]byte("batch"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("batch")}}); err != nil {
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

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}

func TestBlockStoreReadOperationSharesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newWrapperForwardTestStore("test", 0),
		decodedBlocks: decodedBlocks,
	}
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()

	scopedStore, ok := scoped.(*BlockStore)
	if !ok {
		t.Fatalf("scoped store type = %T, want *BlockStore", scoped)
	}
	if scopedStore.GetDecodedBlockCache() != decodedBlocks {
		t.Fatal("scoped read operation did not borrow block-store decoded cache")
	}
}

func TestBlockStoreRmBlockInvalidatesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newWrapperForwardTestStore("test", 0),
		decodedBlocks: decodedBlocks,
	}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after RmBlock error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestBlockStoreBatchTombstoneInvalidatesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newWrapperForwardTestStore("test", 0),
		decodedBlocks: decodedBlocks,
	}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after tombstone batch error = %v, want %v", err, block.ErrNotFound)
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

func TestDirtyTrackingStoreForwardsBatch(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperForwardTestStore("", 0)
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

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{ref1}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 2 {
		t.Fatalf("expected 2 GetBlockExistsBatch calls, got %d", inner.existsBatchHits)
	}
}

func TestDirtyTrackingStoreBatchSkipsExistingBlocks(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperForwardTestStore("", 0)
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
	if _, _, err := inner.PutBlock(ctx, []byte("existing"), nil); err != nil {
		t.Fatalf("seed existing block: %v", err)
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

func TestNewCloudOverlayDoesNotDirtyLowerReads(t *testing.T) {
	ctx := context.Background()
	data := []byte("alpha")
	lower := newProviderSpacewaveTestBlockStore(hash.HashType_HashType_SHA256)
	ref, _, err := lower.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatalf("seed lower block: %v", err)
	}

	upper := newWrapperForwardTestStore("", hash.HashType_HashType_SHA256)
	var dirtyMarks int
	dirtyUpper := &dirtyTrackingStore{
		store: upper,
		markDirty: func(context.Context, *hash.Hash, int64) {
			dirtyMarks++
		},
	}

	overlay := newCloudOverlay(ctx, nil, lower, dirtyUpper)
	got, found, err := overlay.GetBlock(ctx, ref)
	if err != nil {
		t.Fatalf("GetBlock returned error: %v", err)
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("expected lower read hit, got found=%v data=%q", found, string(got))
	}
	if upper.putBlockHits != 0 {
		t.Fatalf("expected no upper writeback on lower read, got %d puts", upper.putBlockHits)
	}
	if dirtyMarks != 0 {
		t.Fatalf("expected no dirty marks from lower read, got %d", dirtyMarks)
	}
}
