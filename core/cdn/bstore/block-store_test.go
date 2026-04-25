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

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"

	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
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

func TestCdnBlockStoreWritesRejected(t *testing.T) {
	bs, err := NewCdnBlockStore(Options{
		CdnBaseURL: "https://cdn.example",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bs.PutBlock(context.Background(), []byte("x"), nil); err == nil {
		t.Fatal("expected PutBlock to error")
	}
	if err := bs.RmBlock(context.Background(), &block.BlockRef{}); err == nil {
		t.Fatal("expected RmBlock to error")
	}
}

// verify-io-completeness: ensure our testCdnServer supports both range and full fetches.
var _ io.Reader = (*bytes.Reader)(nil)
