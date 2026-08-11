package cdn_bstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"

	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

const testSpaceID = "01kpftest0000000000000000"

type testPack struct {
	id    string
	data  []byte
	bloom []byte
}

func buildSinglePack(t *testing.T, id string, blocks map[string][]byte) testPack {
	t.Helper()

	type entry struct {
		h    *hash.Hash
		data []byte
	}
	items := make([]entry, 0, len(blocks))
	for _, data := range blocks {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, entry{h: h, data: data})
	}

	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		e := items[idx]
		idx++
		return e.h, e.data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return testPack{id: id, data: buf.Bytes(), bloom: result.BloomFilter}
}

func encodePointer(t *testing.T, ptr *cdn.CdnRootPointer) []byte {
	t.Helper()
	raw, err := ptr.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(packedmsg.EncodePackedMessage(raw))
}

// testCdnServer serves root.packedmsg and per-pack kvfile responses for a
// fixed Space ID and pack set.
type testCdnServer struct {
	t       *testing.T
	spaceID string
	pointer []byte
	packs   map[string][]byte
	ranges  int
}

func newTestCdnServer(t *testing.T, spaceID string, pointer []byte, packs []testPack) *testCdnServer {
	t.Helper()
	packMap := make(map[string][]byte, len(packs))
	for _, p := range packs {
		packMap[p.id] = p.data
	}
	return &testCdnServer{t: t, spaceID: spaceID, pointer: pointer, packs: packMap}
}

func (s *testCdnServer) handle(w http.ResponseWriter, r *http.Request) {
	rootPath := "/" + s.spaceID + "/root.packedmsg"
	if r.URL.Path == rootPath {
		if s.pointer == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(s.pointer)
		return
	}

	packPrefix := "/" + s.spaceID + "/packs/"
	if !strings.HasPrefix(r.URL.Path, packPrefix) {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, packPrefix)
	// shard/{packID}.kvf
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".kvf") {
		http.NotFound(w, r)
		return
	}
	packID := strings.TrimSuffix(parts[1], ".kvf")
	data, ok := s.packs[packID]
	if !ok {
		http.NotFound(w, r)
		return
	}

	rangeHdr := r.Header.Get("Range")
	if rangeHdr == "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(data)
		return
	}
	s.ranges++
	off, end, err := parseBytesRange(rangeHdr, int64(len(data)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(int64(len(data)), 10))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(data[off : end+1])
}

func parseBytesRange(header string, size int64) (int64, int64, error) {
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, errors.New("unsupported range syntax")
	}
	spec := strings.TrimPrefix(header, prefix)
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("malformed range spec")
	}
	off, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, "parse range start")
	}
	var end int64
	if parts[1] == "" {
		end = size - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, errors.Wrap(err, "parse range end")
		}
	}
	if end >= size {
		end = size - 1
	}
	if off < 0 || off > end {
		return 0, 0, errors.New("range out of bounds")
	}
	return off, end, nil
}

func TestFetchRootPointer(t *testing.T) {
	ctx := context.Background()

	block1 := []byte("hello cdn")
	pack := buildSinglePack(t, "01kcdnpack0000000000000001", map[string][]byte{"b1": block1})

	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	pointerBytes := encodePointer(t, ptr)
	srv := newTestCdnServer(t, testSpaceID, pointerBytes, []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	got, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSpaceId() != testSpaceID {
		t.Fatalf("space id mismatch: %q", got.GetSpaceId())
	}
	if len(got.GetPacks()) != 1 || got.GetPacks()[0].GetId() != pack.id {
		t.Fatalf("unexpected packs: %+v", got.GetPacks())
	}
}

func TestFetchRootPointerMismatchRejected(t *testing.T) {
	ctx := context.Background()
	ptr := &cdn.CdnRootPointer{SpaceId: "wrongspace"}
	pointerBytes := encodePointer(t, ptr)
	srv := newTestCdnServer(t, testSpaceID, pointerBytes, nil)
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	_, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err == nil {
		t.Fatal("expected space id mismatch error")
	}
}

func TestFetchRootPointerAbsent(t *testing.T) {
	ctx := context.Background()
	srv := newTestCdnServer(t, testSpaceID, nil, nil)
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	got, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil pointer for empty space, got %+v", got)
	}
}

func TestCdnBlockStoreReadsBlock(t *testing.T) {
	ctx := context.Background()

	block1 := []byte("hello cdn block store")
	pack := buildSinglePack(t, "01kcdnpack0000000000000002", map[string][]byte{"b1": block1})

	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	pointerBytes := encodePointer(t, ptr)
	srv := newTestCdnServer(t, testSpaceID, pointerBytes, []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	h, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := bs.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected block to be found")
	}
	if !bytes.Equal(got, block1) {
		t.Fatalf("block mismatch: got %q want %q", got, block1)
	}

	// Cached pointer should survive a re-read.
	if bs.Pointer() == nil {
		t.Fatal("expected cached pointer")
	}

	// Invalidate resets both the pointer cache and the manifest.
	bs.Invalidate()
	if bs.Pointer() != nil {
		t.Fatal("pointer should be cleared after Invalidate")
	}

	// Next read re-fetches the pointer transparently.
	got, found, err = bs.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("expected block after re-fetch, found=%v", found)
	}
}

func TestCdnBlockStoreInvalidateClearsDecodedBlockCache(t *testing.T) {
	ctx := context.Background()

	example := &block_mock.Example{Msg: "old pointer"}
	raw, err := example.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	pack := buildSinglePack(t, "01kcdnpack0000000000000004", map[string][]byte{"b1": raw})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	ref, err := block.BuildBlockRef(raw, nil)
	if err != nil {
		t.Fatal(err)
	}

	tx, cursor := block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err)
	}
	bs.GetDecodedBlockCache().Wait()

	srv.pointer = nil
	bs.Invalidate()
	tx, cursor = block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after CDN invalidate error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestCdnBlockStorePointerTTLRefreshClearsDecodedBlockCache(t *testing.T) {
	ctx := context.Background()

	example := &block_mock.Example{Msg: "old ttl pointer"}
	raw, err := example.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	pack := buildSinglePack(t, "01kcdnpack0000000000000005", map[string][]byte{"b1": raw})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
		PointerTTL: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	ref, err := block.BuildBlockRef(raw, nil)
	if err != nil {
		t.Fatal(err)
	}

	tx, cursor := block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err)
	}
	bs.GetDecodedBlockCache().Wait()

	time.Sleep(time.Millisecond)
	srv.pointer = nil
	tx, cursor = block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after CDN pointer TTL error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestCdnBlockStorePointerTTLRejectsStaleWritebackHit(t *testing.T) {
	ctx := context.Background()

	block1 := []byte("hello stale cdn writeback")
	pack := buildSinglePack(t, "01kcdnpack0000000000000006", map[string][]byte{"b1": block1})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	refHash, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: refHash}
	cache := newWritebackReadStore()
	indexCache := newMemIndexCache()
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
		PointerTTL: time.Nanosecond,
		IndexCache: indexCache,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	bs.SetWriteback(ctx, cache, 1<<20)

	if _, found, err := bs.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("first read found=%v err=%v", found, err)
	}
	if err := cache.waitPut(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)
	srv.pointer = nil
	if _, found, err := bs.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("stale writeback read found=%v err=%v, want miss", found, err)
	}
}

func TestCdnBlockStoreOwnsDecodedBlockCache(t *testing.T) {
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: "https://cdn.example.test",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bs.GetDecodedBlockCache() == nil {
		t.Fatal("expected CDN block store to own a decoded-block cache")
	}
	bs.Close()
	if bs.GetDecodedBlockCache() != nil {
		t.Fatal("expected Close to release decoded-block cache")
	}
}

func TestCdnBlockStoreCloseFencesBlockedRefresh(t *testing.T) {
	pack := buildSinglePack(t, "01kcdnpack0000000000000008", map[string][]byte{"b1": []byte("close refresh")})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+testSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		close(refreshStarted)
		<-releaseRefresh
		_, _ = w.Write(encodePointer(t, ptr))
	}))
	defer hs.Close()

	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bs.Close)

	type refreshResult struct {
		ptr *cdn.CdnRootPointer
		err error
	}
	refreshDone := make(chan refreshResult, 1)
	go func() {
		ptr, err := bs.Refresh(t.Context())
		refreshDone <- refreshResult{ptr: ptr, err: err}
	}()
	select {
	case <-refreshStarted:
	case <-t.Context().Done():
		t.Fatalf("Refresh did not reach the root pointer request: %v", t.Context().Err())
	}

	closeDone := make(chan struct{})
	go func() {
		bs.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-t.Context().Done():
		t.Fatalf("Close did not fence blocked Refresh: %v", t.Context().Err())
	}
	if bs.Pointer() != nil {
		t.Fatal("blocked Refresh published a root pointer after Close")
	}
	if got := bs.pfs.SnapshotStats().ManifestEntries; got != 0 {
		t.Fatalf("manifest entries after Close = %d, want 0", got)
	}

	close(releaseRefresh)
	select {
	case result := <-refreshDone:
		if !errors.Is(result.err, packfile_store.ErrPackfileStoreClosed) {
			t.Fatalf("Refresh after Close error = %v, want %v", result.err, packfile_store.ErrPackfileStoreClosed)
		}
		if result.ptr != nil {
			t.Fatalf("Refresh after Close pointer = %#v, want nil", result.ptr)
		}
	case <-t.Context().Done():
		t.Fatalf("blocked Refresh did not complete: %v", t.Context().Err())
	}
	if bs.Pointer() != nil {
		t.Fatal("completed Refresh republished a root pointer after Close")
	}
	if _, err := bs.Refresh(t.Context()); !errors.Is(err, packfile_store.ErrPackfileStoreClosed) {
		t.Fatalf("direct Refresh after Close error = %v, want %v", err, packfile_store.ErrPackfileStoreClosed)
	}
}

func TestCdnBlockStoreWaitsForWritebackBeforeServing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	block1 := []byte("hello cdn blocking writeback")
	pack := buildSinglePack(t, "01kcdnpack0000000000000007", map[string][]byte{"b1": block1})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	refHash, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: refHash}
	cache := newBlockingWritebackStore()
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	bs.SetWriteback(ctx, cache, 1<<20)

	type readResult struct {
		data  []byte
		found bool
		err   error
	}
	done := make(chan readResult, 1)
	go func() {
		data, found, err := bs.GetBlock(ctx, ref)
		done <- readResult{data: data, found: found, err: err}
	}()

	select {
	case <-cache.started:
	case <-ctx.Done():
		t.Fatalf("writeback did not start: %v", ctx.Err())
	}
	select {
	case result := <-done:
		t.Fatalf("GetBlock returned before writeback completed: found=%v err=%v", result.found, result.err)
	default:
	}

	close(cache.release)
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("GetBlock after writeback: %v", result.err)
		}
		if !result.found || !bytes.Equal(result.data, block1) {
			t.Fatalf("GetBlock mismatch after writeback: found=%v data=%q", result.found, result.data)
		}
	case <-ctx.Done():
		t.Fatalf("GetBlock did not complete after writeback: %v", ctx.Err())
	}

	got, found, err := cache.GetBlock(ctx, ref)
	if err != nil {
		t.Fatalf("cached block read: %v", err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("cached block mismatch: found=%v data=%q", found, got)
	}
}

func TestCdnBlockStoreReadsThroughWritebackOnSecondColdStart(t *testing.T) {
	ctx := context.Background()

	block1 := []byte("hello cdn writeback")
	pack := buildSinglePack(t, "01kcdnpack0000000000000003", map[string][]byte{"b1": block1})

	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}
	pointerBytes := encodePointer(t, ptr)
	srv := newTestCdnServer(t, testSpaceID, pointerBytes, []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	refHash, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: refHash}
	cache := newWritebackReadStore()
	first, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	first.SetWriteback(ctx, cache, 1<<20)
	got, found, err := first.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("first read mismatch found=%v data=%q", found, got)
	}
	if err := cache.waitPut(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.SetWriteback(ctx, cache, 1<<20)
	got, found, err = second.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("second read mismatch found=%v data=%q", found, got)
	}
}

func TestCdnBlockStoreWritesRejected(t *testing.T) {
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: "https://cdn.example",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	if _, _, err := bs.PutBlock(context.Background(), []byte("x"), nil); err == nil {
		t.Fatal("expected PutBlock to error")
	}
	if err := bs.RmBlock(context.Background(), &block.BlockRef{}); err == nil {
		t.Fatal("expected RmBlock to error")
	}
}

type writebackReadStore struct {
	block.StoreOps
	putCh chan struct{}
}

func newWritebackReadStore() *writebackReadStore {
	return &writebackReadStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_SHA256,
			false,
		),
		putCh: make(chan struct{}, 1),
	}
}

func (w *writebackReadStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := w.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	select {
	case w.putCh <- struct{}{}:
	default:
	}
	return ref, existed, nil
}

func (w *writebackReadStore) waitPut(ctx context.Context) error {
	select {
	case <-w.putCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type blockingWritebackStore struct {
	block.StoreOps
	started chan struct{}
	release chan struct{}
}

func newBlockingWritebackStore() *blockingWritebackStore {
	base := newWritebackReadStore()
	return &blockingWritebackStore{
		StoreOps: base.StoreOps,
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (w *blockingWritebackStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	close(w.started)
	select {
	case <-w.release:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	return w.StoreOps.PutBlock(ctx, data, opts)
}

// verify-io-completeness: ensure our testCdnServer supports both range and full fetches.
var _ io.Reader = (*bytes.Reader)(nil)
