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

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

type wrapperForwardTestStore struct {
	block.StoreOps
	id                string
	putBlockHits      int
	getBlockHits      int
	putBlockBatchHits int
	existsBatchHits   int
}

type networkLookupTestStore struct {
	block.StoreOps
}

func (s *networkLookupTestStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
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

func (s *wrapperForwardTestStore) GetBlock(
	ctx context.Context,
	ref *block.BlockRef,
) ([]byte, bool, error) {
	s.getBlockHits++
	return s.StoreOps.GetBlock(ctx, ref)
}

// TestBstoreTrackerPublicBaseURL checks that only a public Space's block
// store reads its published files.
func TestBstoreTrackerPublicBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta *api.SpaceMetadataResponse
		want string
	}{
		{"public space", &api.SpaceMetadataResponse{ObjectType: "space", PublicRead: true, PublicBaseUrl: "https://pub.test/file/public"}, "https://pub.test/file/public"},
		{"private space", &api.SpaceMetadataResponse{ObjectType: "space"}, ""},
		{"public other object", &api.SpaceMetadataResponse{ObjectType: "org", PublicRead: true, PublicBaseUrl: "https://pub.test/file/public"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acc := &ProviderAccount{}
			acc.SetSharedObjectMetadata("space-1", tc.meta)
			tracker := &bstoreTracker{a: acc, id: "space-1"}
			if got := tracker.publicBaseURL(context.Background()); got != tc.want {
				t.Fatalf("public base URL %q, want %q", got, tc.want)
			}
		})
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

	remote := newPublicReadRemote(srv.Client(), srv.URL, "space-1", nil, nil)
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

func TestPublicReadRemoteRefreshInvalidatesDecodedCache(t *testing.T) {
	ctx := context.Background()
	ptr := &alpha_cdn.CdnRootPointer{
		SpaceId: "space-1",
		Packs: []*packfile.PackfileEntry{{
			Id:         "pack-2",
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

	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	inner := newWrapperForwardTestStore("test", 0)
	store := &BlockStore{
		store:         inner,
		decodedBlocks: decodedBlocks,
	}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "cached"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if err := inner.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatalf("expected stale decoded cache hit before refresh, got %v", err)
	}

	remote := newPublicReadRemote(srv.Client(), srv.URL, "space-1", nil, decodedBlocks)
	if err := remote.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after public-read refresh error = %v, want %v", err, block.ErrNotFound)
	}
}

type testPublicReadRemoteRefresher struct {
	started chan struct{}
	release chan struct{}
}

func (r *testPublicReadRemoteRefresher) Refresh(ctx context.Context) error {
	r.started <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func TestPublicReadCdnRootRefreshCallbackWiring(t *testing.T) {
	ctx := t.Context()

	remote := &testPublicReadRemoteRefresher{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	acc := &ProviderAccount{}
	tracker := &bstoreTracker{a: acc, id: "space-1"}
	release := tracker.registerPublicReadCdnRootRefresh(
		ctx,
		logrus.New().WithField("test", t.Name()),
		remote,
	)

	acc.fireCdnRootChanged("other-space")
	assertNoPublicReadRefresh(t, remote.started)

	acc.fireCdnRootChanged("space-1")
	waitPublicReadRefresh(t, remote.started)

	returned := make(chan struct{})
	go func() {
		acc.fireCdnRootChanged("space-1")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("cdn-root callback blocked behind an in-flight refresh")
	}
	waitPublicReadRefresh(t, remote.started)

	close(remote.release)
	release()
	acc.fireCdnRootChanged("space-1")
	assertNoPublicReadRefresh(t, remote.started)
}

func waitPublicReadRefresh(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for public-read CDN refresh")
	}
}

func assertNoPublicReadRefresh(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
		t.Fatal("unexpected public-read CDN refresh")
	case <-time.After(100 * time.Millisecond):
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

func (s *wrapperForwardTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

var (
	_ block_store.Store = (*wrapperForwardTestStore)(nil)
	_ block.StoreOps    = (*wrapperForwardTestStore)(nil)
)

func TestBlockStoreForwardsNativeOperations(t *testing.T) {
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

	if _, _, err := store.PutBlock(ctx, []byte("hello"), nil); err != nil {
		t.Fatalf("PutBlock failed: %v", err)
	}
	if inner.putBlockHits != 1 {
		t.Fatalf("expected 1 PutBlock call, got %d", inner.putBlockHits)
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
	store := block_store_writeback.NewMarkingStore(
		inner,
		func(_ context.Context, marks []block_store_writeback.Mark) error {
			dirtyMarks += len(marks)
			return nil
		},
	)

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
	if inner.existsBatchHits != 0 {
		t.Fatalf("unexpected advisory existence calls: %d", inner.existsBatchHits)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{ref1}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 explicit GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}

func TestDirtyTrackingStoreBatchRepairsExistingBlocks(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperForwardTestStore("", 0)
	var dirty []string
	store := block_store_writeback.NewMarkingStore(
		inner,
		func(_ context.Context, marks []block_store_writeback.Mark) error {
			for _, mark := range marks {
				dirty = append(dirty, mark.Hash.MarshalString())
			}
			return nil
		},
	)

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
	if inner.existsBatchHits != 0 {
		t.Fatalf("unexpected GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
	want := []string{existing.GetHash().MarshalString(), fresh.GetHash().MarshalString()}
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

// TestPackReadGrants checks that range reads reuse a live read grant and ask
// for a new one after a refusal.
func TestPackReadGrants(t *testing.T) {
	// Identify the session, block store and pack.
	priv, pid := generateTestKeypair(t)
	resourceID, packID := "01kny7hn4wp25f7t86xzww6bd6", "pack"
	data := []byte("0123456789abcdef")

	// Count grants and reads on a test server.
	var grants, reads int
	var refuse int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Grant reads of /pack?grant=<n> for the nth grant. The routes are exact
	// paths, which the goscript ServeMux also matches.
	mux.HandleFunc("/api/bstore/"+resourceID+"/read", func(w http.ResponseWriter, r *http.Request) {
		// Require the session signature.
		if r.Header.Get("X-Signature") == "" {
			t.Fatal("grant request is not signed")
		}

		// Answer the next grant.
		grants++
		resp := &packfile.ReadResponse{Grants: []*packfile.ReadGrant{{
			PackId:    packID,
			Url:       srv.URL + "/pack?grant=" + strconv.Itoa(grants),
			ExpiresAt: timestamppb.New(time.Now().Add(10 * time.Minute)),
		}}}
		data, err := resp.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	})

	// Serve ranges of the latest grant, refusing one read when asked.
	mux.HandleFunc("/pack", func(w http.ResponseWriter, r *http.Request) {
		// Refuse a read with session credentials, a stale grant or a
		// requested refusal.
		reads++
		if r.Header.Get("X-Signature") != "" {
			t.Fatal("range read carries the session signature")
		}
		if r.URL.Query().Get("grant") != strconv.Itoa(grants) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if refuse != 0 {
			w.WriteHeader(refuse)
			refuse = 0
			return
		}

		// Serve the range.
		start, end, ok := parseHTTPRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Fatalf("missing or invalid Range header: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start:end])
	})

	// Open readers that share the session's grants.
	sessionCli := NewSessionClient(srv.Client(), srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	newReader := func() io.ReaderAt {
		return packfile_store.NewHTTPRangeReader(
			srv.Client(),
			"",
			int64(len(data)),
			4,
			func(req *http.Request) error {
				return sessionCli.grantPackRead(req, resourceID, packID)
			},
			func(resp *http.Response) {
				sessionCli.observePackRead(resp, resourceID, packID)
			},
		).ReaderAt(context.Background())
	}

	// Two reads share one grant.
	rd, buf := newReader(), make([]byte, 4)
	for _, off := range []int64{0, 8} {
		if _, err := rd.ReadAt(buf, off); err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d): %v", off, err)
		}
	}
	if grants != 1 || reads != 2 || string(buf) != "89ab" {
		t.Fatalf("grants %d, reads %d, data %q", grants, reads, buf)
	}

	// A read refused for an expired authorization, or for a pack that moved
	// to the public bucket, drops the grant, and the next read asks for
	// another.
	for i, status := range []int{http.StatusUnauthorized, http.StatusNotFound} {
		rd, refuse = newReader(), status
		if _, err := rd.ReadAt(buf, 12); err == nil {
			t.Fatalf("read refused with %d succeeded", status)
		}
		if _, err := rd.ReadAt(buf, 12); err != nil && err != io.EOF {
			t.Fatalf("read after refusal with %d: %v", status, err)
		}
		if grants != i+2 || string(buf) != "cdef" {
			t.Fatalf("after %d: grants %d, data %q", status, grants, buf)
		}
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

func TestCloudBlockStoreBucketKeepsAccountCacheLocalOnly(t *testing.T) {
	t.Parallel()

	tracker := &bstoreTracker{
		a:  &ProviderAccount{accountID: "account"},
		id: "block-store",
	}
	conf, err := tracker.buildBucketConf()
	if err != nil {
		t.Fatal(err)
	}
	if got := conf.GetRev(); got != blockStoreBucketConfigRev {
		t.Fatalf("bucket config revision = %d, want %d", got, blockStoreBucketConfigRev)
	}
	var lookupConf lookup_concurrent.Config
	if err := lookupConf.UnmarshalJSON(conf.GetLookup().GetController().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if lookupConf.GetNotFoundBehavior() != lookup_concurrent.NotFoundBehavior_NotFoundBehavior_NONE ||
		lookupConf.GetPutBlockBehavior() != lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL ||
		lookupConf.GetWritebackBehavior() != lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL {
		t.Fatalf("unexpected account cache lookup policy: %+v", lookupConf)
	}
}

func TestCloudOverlayFallsThroughOnceAndRecordsCloudSource(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	acc := &ProviderAccount{}
	data := []byte("cloud")
	lower := newWrapperForwardTestStore("lower", hash.HashType_HashType_SHA256)
	ref, _, err := lower.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	upper := newWrapperForwardTestStore("upper", hash.HashType_HashType_SHA256)
	sourceUpper := &sourceTrackingStore{
		StoreOps:   upper,
		account:    acc,
		bstoreID:   "block-store",
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	sourceLower := &sourceTrackingStore{
		StoreOps: lower,
		account:  acc,
		bstoreID: "block-store",
		source:   SyncTelemetryBlockSourceCloud,
	}

	got, found, err := newCloudOverlay(ctx, nil, sourceLower, sourceUpper).GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("Cloud fallback = %q/%v, want %q/true", got, found, data)
	}
	if upper.getBlockHits != 1 || lower.getBlockHits != 1 {
		t.Fatalf("source reads = upper:%d lower:%d, want 1/1", upper.getBlockHits, lower.getBlockHits)
	}
	telemetry := acc.GetSyncTelemetrySnapshot()
	if len(telemetry.BlockStores) != 1 ||
		telemetry.BlockStores[0].CloudHitCount != 1 ||
		telemetry.BlockStores[0].DirectHitCount != 0 ||
		telemetry.BlockStores[0].LastSource != SyncTelemetryBlockSourceCloud {
		t.Fatalf("unexpected source telemetry: %+v", telemetry.BlockStores)
	}
}

func TestCloudOverlayRecordsDirectDemandSource(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	acc := &ProviderAccount{}
	network := newProviderSpacewaveTestBlockStore(hash.HashType_HashType_SHA256)
	data := []byte("direct")
	ref, _, err := network.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceUpper := &sourceTrackingStore{
		StoreOps:   &networkLookupTestStore{StoreOps: network},
		account:    acc,
		bstoreID:   "block-store",
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}

	got, found, err := sourceUpper.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("direct read = %q/%v, want %q/true", got, found, data)
	}
	telemetry := acc.GetSyncTelemetrySnapshot()
	if len(telemetry.BlockStores) != 1 ||
		telemetry.BlockStores[0].DirectHitCount != 1 ||
		telemetry.BlockStores[0].CloudHitCount != 0 ||
		telemetry.BlockStores[0].LastSource != SyncTelemetryBlockSourceDirect {
		t.Fatalf("unexpected source telemetry: %+v", telemetry.BlockStores)
	}
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
	dirtyUpper := block_store_writeback.NewMarkingStore(
		upper,
		func(_ context.Context, marks []block_store_writeback.Mark) error {
			dirtyMarks += len(marks)
			return nil
		},
	)

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
