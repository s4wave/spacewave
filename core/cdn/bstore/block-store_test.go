package cdn_bstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
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
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

const testSpaceID = "01kpftest0000000000000000"

type testPack struct {
	id    string
	data  []byte
	bloom []byte
}

type doneObservedContext struct {
	context.Context
	observed chan struct{}
}

func (c doneObservedContext) Done() <-chan struct{} {
	select {
	case c.observed <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

func buildSinglePack(t *testing.T, id string, blocks map[string][]byte) testPack {
	// Hash the supplied blocks into packfile entries for the test.
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

	// Write the block entries into one packfile with its bloom filter.
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		e := items[idx]
		idx++
		return e.h, &block.StoredBlock{Data: e.data, RefsKnown: true}, nil
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
	// Serve the current root pointer or report an empty CDN Space.
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

	// Require a pack URL within the test CDN Space.
	packPrefix := "/" + s.spaceID + "/packs/"
	if !strings.HasPrefix(r.URL.Path, packPrefix) {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, packPrefix)

	// Resolve the requested shard and pack ID to stored pack bytes.
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

	// Serve the complete pack when the request has no byte range.
	rangeHdr := r.Header.Get("Range")
	if rangeHdr == "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(data)
		return
	}

	// Validate the requested pack range and return its partial response.
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
	// Require a byte-range header with one start and end pair.
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, errors.New("unsupported range syntax")
	}
	spec := strings.TrimPrefix(header, prefix)
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("malformed range spec")
	}

	// Decode the starting byte offset of the requested range.
	off, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, "parse range start")
	}

	// Resolve the range end, using the last pack byte for an open range.
	var end int64
	if parts[1] == "" {
		end = size - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, errors.Wrap(err, "parse range end")
		}
	}

	// Constrain the requested range to the available pack bytes.
	if end >= size {
		end = size - 1
	}
	if off < 0 || off > end {
		return 0, 0, errors.New("range out of bounds")
	}
	return off, end, nil
}

func TestFetchRootPointer(t *testing.T) {
	// Build a packfile containing the root-pointer test block.
	ctx := context.Background()
	block1 := []byte("hello cdn")
	pack := buildSinglePack(t, "01kcdnpack0000000000000001", map[string][]byte{"b1": block1})

	// Serve a root pointer that lists the test packfile.
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

	// Fetch the CDN root pointer from the test server.
	got, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the root pointer identifies the requested Space and packfile.
	if got.GetSpaceId() != testSpaceID {
		t.Fatalf("space id mismatch: %q", got.GetSpaceId())
	}
	if len(got.GetPacks()) != 1 || got.GetPacks()[0].GetId() != pack.id {
		t.Fatalf("unexpected packs: %+v", got.GetPacks())
	}
}

func TestFetchRootPointerMismatchRejected(t *testing.T) {
	// Serve a root pointer belonging to a different Space.
	ctx := context.Background()
	ptr := &cdn.CdnRootPointer{SpaceId: "wrongspace"}
	pointerBytes := encodePointer(t, ptr)
	srv := newTestCdnServer(t, testSpaceID, pointerBytes, nil)
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	// Verify the CDN fetch rejects the mismatched Space ID.
	_, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err == nil {
		t.Fatal("expected space id mismatch error")
	}
}

func TestFetchRootPointerAbsent(t *testing.T) {
	// Serve an empty CDN Space without a root pointer.
	ctx := context.Background()
	srv := newTestCdnServer(t, testSpaceID, nil, nil)
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	// Fetch the absent CDN pointer without a transport error.
	got, err := FetchRootPointer(ctx, hs.Client(), hs.URL, testSpaceID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the absent pointer represents an empty Space.
	if got != nil {
		t.Fatalf("expected nil pointer for empty space, got %+v", got)
	}
}

// TestCdnBlockStoreRootPointerBaseURL reads the pointer from the owner's
// server under a path prefix while pack ranges stay on the CDN.
func TestCdnBlockStoreRootPointerBaseURL(t *testing.T) {
	// Build one pack and a pointer that lists it.
	ctx := context.Background()
	block1 := []byte("hello root pointer origin")
	pack := buildSinglePack(t, "01kcdnpack0000000000000009", map[string][]byte{"b1": block1})
	ptr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}

	// The CDN serves packs but no pointer; the owner serves only the pointer.
	packSrv := newTestCdnServer(t, testSpaceID, nil, []testPack{pack})
	packHS := httptest.NewServer(http.HandlerFunc(packSrv.handle))
	defer packHS.Close()
	rootSrv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), nil)
	rootHS := httptest.NewServer(http.StripPrefix("/cdn", http.HandlerFunc(rootSrv.handle)))
	defer rootHS.Close()

	// Point the store at the CDN for packs and at the owner for the pointer.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL:         packHS.URL,
		RootPointerBaseURL: rootHS.URL + "/cdn/",
		SpaceID:            testSpaceID,
		HttpClient:         packHS.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	// Read the block through the owner's pointer and the CDN's pack.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := bs.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("block mismatch: found=%v got %q want %q", found, got, block1)
	}
}

func TestCdnBlockStoreReadsBlock(t *testing.T) {
	// Build the block and packfile for CDN readback.
	ctx := context.Background()
	block1 := []byte("hello cdn block store")
	pack := buildSinglePack(t, "01kcdnpack0000000000000002", map[string][]byte{"b1": block1})

	// Serve a root pointer that exposes the test block packfile.
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

	// Open a CDN block store against the test server.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	// Read the test block through its content hash.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := bs.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the CDN block bytes match the packed block.
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
	// Serve a packed example block through the initial CDN pointer.
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

	// Serve the example pack through its initial root pointer.
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	// Open the CDN block store and identify the packed example.
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

	// Populate the decoded-block cache by unmarshaling the example.
	tx, cursor := block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err)
	}
	bs.GetDecodedBlockCache().Wait()

	// Invalidate the removed CDN pointer before decoding the example again.
	srv.pointer = nil
	bs.Invalidate()
	tx, cursor = block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())

	// Verify invalidation prevents reuse of the cached example.
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after CDN invalidate error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestCdnBlockStoreCanceledPointerLeaderDoesNotPoisonFollower(t *testing.T) {
	// Serve a root pointer while blocking the first request until cancellation.
	ptr := &cdn.CdnRootPointer{SpaceId: testSpaceID}
	pointerBytes := encodePointer(t, ptr)
	var requests atomic.Int64
	leaderStarted := make(chan struct{})
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+testSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) == 1 {
			close(leaderStarted)
			<-r.Context().Done()
			return
		}
		_, _ = w.Write(pointerBytes)
	}))
	defer hs.Close()

	// Open a CDN block store against the cancelable pointer server.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	// Start a pointer leader with a deadline and wait for its request.
	leaderCtx, cancelLeader := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancelLeader()
	leaderDone := make(chan error, 1)
	go func() {
		_, err := bs.Refresh(leaderCtx)
		leaderDone <- err
	}()
	select {
	case <-leaderStarted:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Join the pointer request with a healthy follower.
	followerDone := make(chan error, 1)
	go func() {
		_, err := bs.Refresh(t.Context())
		followerDone <- err
	}()

	// Cancel a separate follower without interrupting the shared pointer request.
	callerCtx, cancelCaller := context.WithCancel(t.Context())
	callerDone := make(chan error, 1)
	go func() {
		_, err := bs.Refresh(callerCtx)
		callerDone <- err
	}()
	cancelCaller()
	select {
	case err := <-callerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled follower error = %v, want %v", err, context.Canceled)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Verify the expired leader permits a healthy follower to publish the pointer.
	select {
	case err := <-leaderDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("leader error = %v, want %v", err, context.DeadlineExceeded)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	select {
	case err := <-followerDone:
		if err != nil {
			t.Fatalf("healthy follower refresh: %v", err)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("root pointer requests = %d, want canceled leader plus replacement", got)
	}
	if got := bs.Pointer(); got == nil || !got.EqualVT(ptr) {
		t.Fatalf("root pointer = %#v, want %#v", got, ptr)
	}
}

func TestCdnBlockStoreSharesNonContextPointerError(t *testing.T) {
	// Serve one gated CDN pointer request that returns an unavailable response.
	var requests atomic.Int64
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Require the requested root pointer path before admitting the CDN request.
		if r.URL.Path != "/"+testSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}

		// Hold the CDN request until all readers can share its failure.
		requests.Add(1)
		close(requestStarted)
		<-releaseRequest
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer hs.Close()

	// Open the CDN block store against the failing pointer server.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	// Start the pointer leader and wait for its request to reach the server.
	const readers = 32
	done := make(chan error, readers)
	go func() {
		_, err := bs.Refresh(t.Context())
		done <- err
	}()
	select {
	case <-requestStarted:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Admit the remaining pointer readers to the shared request.
	admitted := make([]chan struct{}, 0, readers-1)
	for range readers - 1 {
		observed := make(chan struct{}, 1)
		admitted = append(admitted, observed)
		go func() {
			ctx := doneObservedContext{Context: t.Context(), observed: observed}
			_, err := bs.Refresh(ctx)
			done <- err
		}()
	}
	for _, observed := range admitted {
		select {
		case <-observed:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}

	// Release the failed request and verify every reader receives the shared error.
	close(releaseRequest)
	for range readers {
		if err := <-done; err == nil || !strings.Contains(err.Error(), "status 503") {
			t.Fatalf("shared root pointer error = %v, want status 503", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("root pointer requests = %d, want one shared request", got)
	}
}

func TestCdnBlockStoreCoalescesUnchangedPointerRefreshes(t *testing.T) {
	// Serve a stable CDN pointer while gating later refresh requests.
	ptr := &cdn.CdnRootPointer{SpaceId: testSpaceID}
	pointerBytes := encodePointer(t, ptr)
	var requests atomic.Int64
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Require the requested root pointer path before handling a refresh.
		if r.URL.Path != "/"+testSpaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}

		// Count CDN pointer requests and gate refreshes after the initial fetch.
		request := requests.Add(1)
		if request == 2 {
			close(refreshStarted)
		}
		if request > 1 {
			<-releaseRefresh
		}
		_, _ = w.Write(pointerBytes)
	}))
	defer hs.Close()

	// Open the CDN block store and fetch its initial pointer.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()
	if _, err := bs.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Start the refresh leader and wait for its request to reach the server.
	const readers = 32
	done := make(chan error, readers)
	go func() {
		_, err := bs.Refresh(t.Context())
		done <- err
	}()
	select {
	case <-refreshStarted:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Admit the remaining readers to the unchanged pointer refresh.
	admitted := make([]chan struct{}, 0, readers-1)
	for range readers - 1 {
		observed := make(chan struct{}, 1)
		admitted = append(admitted, observed)
		go func() {
			ctx := doneObservedContext{Context: t.Context(), observed: observed}
			_, err := bs.Refresh(ctx)
			done <- err
		}()
	}
	for _, observed := range admitted {
		select {
		case <-observed:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}

	// Release the shared refresh and verify every reader completes.
	close(releaseRefresh)
	for range readers {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("root pointer requests = %d, want initial plus one shared refresh", got)
	}

	// Verify the unchanged CDN pointer preserves its publication epoch.
	bs.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if bs.pointerEpoch != 1 {
			t.Fatalf("unchanged pointer epoch = %d, want 1", bs.pointerEpoch)
		}
	})
}

func TestCdnBlockStorePointerTTLRefreshClearsDecodedBlockCache(t *testing.T) {
	// Serve a packed example block through a pointer with a short lifetime.
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

	// Serve the example pack through its initial root pointer.
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, ptr), []testPack{pack})
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	defer hs.Close()

	// Open the CDN block store with an expiring pointer cache.
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

	// Populate the decoded-block cache from the initial CDN pointer.
	tx, cursor := block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err)
	}
	bs.GetDecodedBlockCache().Wait()

	// Expire the CDN pointer and remove the example from the server.
	time.Sleep(time.Millisecond)
	srv.pointer = nil
	tx, cursor = block.NewTransaction(bs, nil, ref, nil)
	tx.SetDecodedBlockCache(bs.GetDecodedBlockCache())

	// Verify pointer refresh prevents reuse of the decoded example.
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after CDN pointer TTL error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestCdnBlockStorePointerTTLRejectsStaleWritebackHit(t *testing.T) {
	// Serve a packed block through the initial CDN pointer.
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

	// Identify the packed block and allocate its writeback caches.
	refHash, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: refHash}
	cache := newWritebackReadStore()
	indexCache := newMemIndexCache()

	// Open the CDN block store with an expiring pointer and local writeback.
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

	// Read the block and wait for its local writeback to complete.
	if _, found, err := bs.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("first read found=%v err=%v", found, err)
	}
	if err := cache.waitPut(ctx); err != nil {
		t.Fatal(err)
	}

	// Expire the CDN pointer and verify the removed block misses local writeback.
	time.Sleep(time.Millisecond)
	srv.pointer = nil
	if _, found, err := bs.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("stale writeback read found=%v err=%v, want miss", found, err)
	}
}

func TestCdnBlockStoreOwnsDecodedBlockCache(t *testing.T) {
	// Open a CDN block store with its own decoded-block cache.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: "https://cdn.example.test",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the decoded-block cache lives until the CDN block store closes.
	if bs.GetDecodedBlockCache() == nil {
		t.Fatal("expected CDN block store to own a decoded-block cache")
	}
	bs.Close()
	if bs.GetDecodedBlockCache() != nil {
		t.Fatal("expected Close to release decoded-block cache")
	}
}

func TestCdnBlockStoreCloseFencesBlockedRefresh(t *testing.T) {
	// Serve a packed root pointer while holding its refresh response.
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

	// Open the CDN block store whose refresh will be closed in flight.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bs.Close)

	// Start the pointer refresh and wait for its request to reach the server.
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

	// Admit a follower to the blocked CDN pointer refresh.
	followerDone := make(chan error, 1)
	followerAdmitted := make(chan struct{}, 1)
	go func() {
		ctx := doneObservedContext{Context: t.Context(), observed: followerAdmitted}
		_, err := bs.Refresh(ctx)
		followerDone <- err
	}()
	select {
	case <-followerAdmitted:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Close the CDN block store while its root request remains blocked.
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

	// Verify shutdown clears the CDN manifest and wakes the refresh follower.
	if bs.Pointer() != nil {
		t.Fatal("blocked Refresh published a root pointer after Close")
	}
	if got := bs.pfs.SnapshotStats().ManifestEntries; got != 0 {
		t.Fatalf("manifest entries after Close = %d, want 0", got)
	}
	select {
	case err := <-followerDone:
		if !errors.Is(err, packfile_store.ErrPackfileStoreClosed) {
			t.Fatalf("Refresh follower after Close error = %v, want %v", err, packfile_store.ErrPackfileStoreClosed)
		}
	case <-t.Context().Done():
		t.Fatalf("Refresh follower did not observe Close: %v", t.Context().Err())
	}

	// Release the root response and verify shutdown prevents pointer publication.
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

func TestCdnBlockStoreReadsThroughWritebackOnSecondColdStart(t *testing.T) {
	// Build a packfile for writeback reuse across CDN store instances.
	ctx := context.Background()
	block1 := []byte("hello cdn writeback")
	pack := buildSinglePack(t, "01kcdnpack0000000000000003", map[string][]byte{"b1": block1})

	// Serve the root pointer and its writeback test packfile.
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

	// Identify the packed block and allocate a shared writeback store.
	refHash, err := hash.Sum(hash.HashType_HashType_SHA256, block1)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: refHash}
	cache := newWritebackReadStore()

	// Open the first CDN block store with local writeback enabled.
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

	// Read the block and wait for the first store to persist its writeback.
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

	// Open a second CDN block store using the same writeback store.
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

	// Verify the second store reads the same block through local writeback.
	got, found, err = second.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, block1) {
		t.Fatalf("second read mismatch found=%v data=%q", found, got)
	}
}

func TestCdnBlockStoreWritesRejected(t *testing.T) {
	// Open the anonymous CDN block store for write rejection checks.
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: "https://cdn.example",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bs.Close()

	// Verify the anonymous CDN store rejects block writes and removals.
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

// PutBlockBatch writes entries and signals one put.
func (w *writebackReadStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := w.StoreOps.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	select {
	case w.putCh <- struct{}{}:
	default:
	}
	return nil
}

func (w *writebackReadStore) waitPut(ctx context.Context) error {
	select {
	case <-w.putCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// verify-io-completeness: ensure our testCdnServer supports both range and full fetches.
var _ io.Reader = (*bytes.Reader)(nil)
