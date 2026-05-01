package delta

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/net/hash"

	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

// blockSpec describes a block that will be packed into a test kvfile.
type blockSpec struct {
	key  string
	data []byte
}

// memExists is an in-memory ExistsChecker that returns true for any hash
// whose MarshalString is present in the map.
type memExists map[string]bool

func (m memExists) GetBlockExists(_ context.Context, ref *block.BlockRef) (bool, error) {
	return m[ref.GetHash().MarshalString()], nil
}

type testRefGraph struct {
	out map[string][]string
	in  map[string][]string
}

func newTestRefGraph() *testRefGraph {
	return &testRefGraph{
		out: make(map[string][]string),
		in:  make(map[string][]string),
	}
}

func (g *testRefGraph) add(subject, object string) {
	g.out[subject] = append(g.out[subject], object)
	g.in[object] = append(g.in[object], subject)
}

func (g *testRefGraph) GetOutgoingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.out[node]), nil
}

func (g *testRefGraph) GetIncomingRefs(_ context.Context, node string) ([]string, error) {
	return slices.Clone(g.in[node]), nil
}

// buildTestKvfile packs =n= deterministic blocks into a kvfile using PackBlocks.
// Returns the raw bytes, the ordered block specs, and a *kvfile.Reader.
func buildTestKvfile(t *testing.T, prefix string, n int, sizePerBlock int) (specs []blockSpec, reader *kvfile.Reader) {
	t.Helper()
	specs = make([]blockSpec, 0, n)
	for i := range n {
		body := make([]byte, sizePerBlock)
		for j := range body {
			body[j] = byte((i*13 + j) & 0xff)
		}
		body[0] = byte(i)
		if len(prefix) > 0 {
			copy(body, []byte(prefix+"-"+strconv.Itoa(i)))
		}
		h, err := hash.Sum(hash.HashType_HashType_SHA256, body)
		if err != nil {
			t.Fatalf("hash.Sum: %v", err)
		}
		specs = append(specs, blockSpec{key: h.MarshalString(), data: body})
	}

	var buf bytes.Buffer
	w := kvfile.NewWriter(&buf)
	for _, s := range specs {
		if err := w.WriteValue([]byte(s.key), bytes.NewReader(s.data)); err != nil {
			t.Fatalf("write kvfile value: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close kvfile: %v", err)
	}

	rd := bytes.NewReader(buf.Bytes())
	reader, err := kvfile.BuildReader(rd, uint64(buf.Len()))
	if err != nil {
		t.Fatalf("build kvfile reader: %v", err)
	}
	return specs, reader
}

func blockRefFromKey(t *testing.T, key string) *block.BlockRef {
	t.Helper()
	h := &hash.Hash{}
	if err := h.ParseFromB58(key); err != nil {
		t.Fatalf("parse block hash: %v", err)
	}
	return block.NewBlockRef(h)
}

func packPhysicalKeys(t *testing.T, body []byte) []string {
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

// TestDiffBlockStoresEmpty verifies that when every source block already lives
// in the mirror, DiffBlockStores yields an exhausted iterator and
// EmitDeltaChunks emits nothing (empty-diff no-op).
func TestDiffBlockStoresEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	specs, reader := buildTestKvfile(t, "empty", 4, 64)
	mirror := memExists{}
	for _, s := range specs {
		mirror[s.key] = true
	}

	iter, err := DiffBlockStores(ctx, reader, mirror)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	emitCalls := 0
	emitted, err := EmitDeltaChunks(ctx, "test-resource", iter, DefaultMaxChunkBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		emitCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}
	if emitCalls != 0 {
		t.Fatalf("emit called %d times, expected 0", emitCalls)
	}
	if len(emitted) != 0 {
		t.Fatalf("emitted %d entries, expected 0", len(emitted))
	}
}

// TestDiffBlockStoresSingleChunk verifies that a small diff packs into one
// chunk whose PackfileEntry reports the correct block count and non-empty
// bloom filter.
func TestDiffBlockStoresSingleChunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	specs, reader := buildTestKvfile(t, "single", 6, 128)
	// mirror holds the first two specs; the remaining four should be packed.
	mirror := memExists{specs[0].key: true, specs[1].key: true}
	expectedCount := uint64(len(specs) - 2)

	iter, err := DiffBlockStores(ctx, reader, mirror)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	var emitted []*packfile.PackfileEntry
	var chunks [][]byte
	emitted, err = EmitDeltaChunks(ctx, "test-resource", iter, DefaultMaxChunkBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		if idx != len(chunks) {
			t.Fatalf("idx=%d len(chunks)=%d", idx, len(chunks))
		}
		chunks = append(chunks, append([]byte(nil), data...))
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}

	if len(emitted) != 1 {
		t.Fatalf("emitted %d entries, expected 1", len(emitted))
	}
	entry := emitted[0]
	if entry.GetBlockCount() != expectedCount {
		t.Fatalf("block_count=%d expected=%d", entry.GetBlockCount(), expectedCount)
	}
	if len(entry.GetBloomFilter()) == 0 {
		t.Fatal("empty bloom filter")
	}
	if entry.GetId() == "" {
		t.Fatal("empty pack id")
	}
	if entry.GetCreatedAt() == nil {
		t.Fatal("nil created_at")
	}
	if entry.GetSizeBytes() == 0 || uint64(len(chunks[0])) != entry.GetSizeBytes() {
		t.Fatalf("size_bytes=%d chunk_len=%d", entry.GetSizeBytes(), len(chunks[0]))
	}

	// Round-trip the chunk to confirm it contains exactly the expected blocks.
	rd := bytes.NewReader(chunks[0])
	reader2, err := kvfile.BuildReader(rd, uint64(len(chunks[0])))
	if err != nil {
		t.Fatalf("rebuild reader: %v", err)
	}
	if reader2.Size() != expectedCount {
		t.Fatalf("round-trip size=%d expected=%d", reader2.Size(), expectedCount)
	}
	for _, s := range specs[2:] {
		data, found, err := reader2.Get([]byte(s.key))
		if err != nil {
			t.Fatalf("Get %s: %v", s.key, err)
		}
		if !found {
			t.Fatalf("block %s missing from chunk", s.key)
		}
		if !bytes.Equal(data, s.data) {
			t.Fatalf("block %s data mismatch", s.key)
		}
	}
}

func TestDiffBlockStoresWithRefGraphOrdersPhysicalPack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	specs, reader := buildTestKvfile(t, "graph", 4, 128)
	stray := blockRefFromKey(t, specs[0].key)
	rootA := blockRefFromKey(t, specs[1].key)
	rootB := blockRefFromKey(t, specs[2].key)
	childA := blockRefFromKey(t, specs[3].key)

	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("object-b"), block_gc.BlockIRI(rootB))
	graph.add(block_gc.ObjectIRI("object-a"), block_gc.BlockIRI(rootA))
	graph.add(block_gc.BlockIRI(rootA), block_gc.BlockIRI(childA))

	iter, err := DiffBlockStoresWithRefGraph(ctx, reader, nil, graph)
	if err != nil {
		t.Fatalf("DiffBlockStoresWithRefGraph: %v", err)
	}

	var chunks [][]byte
	_, err = EmitDeltaChunks(ctx, "test-resource", iter, DefaultMaxChunkBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		chunks = append(chunks, bytes.Clone(data))
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("emitted %d chunks, want 1", len(chunks))
	}

	want := []string{
		rootA.GetHash().MarshalString(),
		childA.GetHash().MarshalString(),
		rootB.GetHash().MarshalString(),
		stray.GetHash().MarshalString(),
	}
	if got := packPhysicalKeys(t, chunks[0]); !slices.Equal(got, want) {
		t.Fatalf("physical pack order = %v, want %v", got, want)
	}
}

// TestDiffBlockStoresMultiChunk verifies that EmitDeltaChunks rolls into a new
// chunk when adding the next block would push the running byte total past
// maxBytes. Each block is larger than maxBytes/2 so every block lands in its
// own chunk.
func TestDiffBlockStoresMultiChunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const nBlocks = 3
	const blockSize = 1024
	specs, reader := buildTestKvfile(t, "multi", nBlocks, blockSize)

	iter, err := DiffBlockStores(ctx, reader, nil)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	// Force one block per chunk by choosing maxBytes below 2*blockSize.
	maxBytes := int64(blockSize + 256)
	var emitted []*packfile.PackfileEntry
	var sizes []uint64
	emitted, err = EmitDeltaChunks(ctx, "test-resource", iter, maxBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		sizes = append(sizes, entry.GetSizeBytes())
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}
	if len(emitted) != nBlocks {
		t.Fatalf("emitted %d entries, expected %d", len(emitted), nBlocks)
	}
	for i, e := range emitted {
		if e.GetBlockCount() != 1 {
			t.Fatalf("chunk %d block_count=%d expected 1", i, e.GetBlockCount())
		}
		if uint64(maxBytes) < e.GetSizeBytes() {
			t.Fatalf("chunk %d size=%d exceeds maxBytes=%d", i, e.GetSizeBytes(), maxBytes)
		}
	}
	if len(sizes) != nBlocks {
		t.Fatalf("sizes collected=%d expected=%d", len(sizes), nBlocks)
	}
	if specs[0].key == specs[1].key {
		t.Fatal("spec keys collided; check buildTestKvfile determinism")
	}

	seen := make(map[string]bool, len(emitted))
	for _, entry := range emitted {
		if entry.GetId() == "" {
			t.Fatal("empty pack id")
		}
		if seen[entry.GetId()] {
			t.Fatalf("duplicate pack id %q", entry.GetId())
		}
		seen[entry.GetId()] = true
	}
}

func TestEmitDeltaChunksBlockCountCeiling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	nBlocks := int(writer.DefaultMaxBlocksPerPack) + 1
	_, reader := buildTestKvfile(t, "count", nBlocks, 32)
	iter, err := DiffBlockStores(ctx, reader, nil)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	emitted, err := EmitDeltaChunks(ctx, "test-resource", iter, DefaultMaxChunkBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		if len(entry.GetBloomFilter()) == 0 {
			t.Fatalf("chunk %d missing bloom metadata", idx)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted %d entries, expected 2", len(emitted))
	}
	var total uint64
	for i, entry := range emitted {
		if entry.GetBlockCount() > writer.DefaultMaxBlocksPerPack {
			t.Fatalf("chunk %d block_count=%d exceeds ceiling %d", i, entry.GetBlockCount(), writer.DefaultMaxBlocksPerPack)
		}
		total += entry.GetBlockCount()
	}
	if total != uint64(nBlocks) {
		t.Fatalf("total block_count=%d expected=%d", total, nBlocks)
	}
}

// TestOpenMirrorUnionAbsent verifies the mirror-absent degenerate case:
// OpenMirrorUnion returns (nil, nil) when the per-space subdir does not exist.
func TestOpenMirrorUnionAbsent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mirrorDir := t.TempDir()
	// No {mirrorDir}/{spaceID}/ subdir; should degenerate cleanly.
	union, err := OpenMirrorUnion(ctx, nil, mirrorDir, "01test00000000000000000000")
	if err != nil {
		t.Fatalf("OpenMirrorUnion (subdir missing): %v", err)
	}
	if union != nil {
		t.Fatalf("expected nil union, got %+v", union)
	}

	// Space dir exists but no packs/ subdir: still degenerate.
	spaceID := "01test00000000000000000001"
	if err := os.MkdirAll(filepath.Join(mirrorDir, spaceID), 0o755); err != nil {
		t.Fatalf("mkdir space dir: %v", err)
	}
	union, err = OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err != nil {
		t.Fatalf("OpenMirrorUnion (packs missing): %v", err)
	}
	if union != nil {
		t.Fatalf("expected nil union, got %+v", union)
	}

	// packs/ exists but is empty: still degenerate.
	if err := os.MkdirAll(filepath.Join(mirrorDir, spaceID, "packs"), 0o755); err != nil {
		t.Fatalf("mkdir packs dir: %v", err)
	}
	union, err = OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err != nil {
		t.Fatalf("OpenMirrorUnion (packs empty): %v", err)
	}
	if union != nil {
		t.Fatalf("expected nil union, got %+v", union)
	}
}

func TestOpenMirrorUnionReadsRawPackKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	body := []byte("mirror raw pack block")
	h, err := hash.Sum(hash.HashType_HashType_SHA256, body)
	if err != nil {
		t.Fatalf("hash.Sum: %v", err)
	}
	emitted := false
	var pack bytes.Buffer
	if _, err := writer.PackBlocks(&pack, func() (*hash.Hash, []byte, error) {
		if emitted {
			return nil, nil, nil
		}
		emitted = true
		return h, body, nil
	}); err != nil {
		t.Fatalf("PackBlocks: %v", err)
	}

	mirrorDir := t.TempDir()
	spaceID := "01test000000000000rawkeys"
	packDir := filepath.Join(mirrorDir, spaceID, "packs", "01")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "01PACK.kvf"), pack.Bytes(), 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	union, err := OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err != nil {
		t.Fatalf("OpenMirrorUnion: %v", err)
	}
	defer union.Close()

	got, found, err := union.GetBlock(ctx, block.NewBlockRef(h))
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("raw-key mirror pack did not contain block")
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("block body mismatch: got %q want %q", got, body)
	}
}

// TestOpenMirrorUnionSpaceIDMismatch verifies that a =root.packedmsg= whose
// embedded =CdnRootPointer.space_id= does not match =spaceID= is a fatal error
// before any packs are opened.
func TestOpenMirrorUnionSpaceIDMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mirrorDir := t.TempDir()
	spaceID := "01test00000000000000abcd00"
	otherID := "01test00000000000000wxyz00"
	spaceDir := filepath.Join(mirrorDir, spaceID)
	if err := os.MkdirAll(filepath.Join(spaceDir, "packs"), 0o755); err != nil {
		t.Fatalf("mkdir packs: %v", err)
	}

	ptr := &alpha_cdn.CdnRootPointer{SpaceId: otherID}
	body, err := ptr.MarshalVT()
	if err != nil {
		t.Fatalf("marshal ptr: %v", err)
	}
	encoded := packedmsg.EncodePackedMessage(body)
	if err := os.WriteFile(filepath.Join(spaceDir, "root.packedmsg"), []byte(encoded), 0o644); err != nil {
		t.Fatalf("write root.packedmsg: %v", err)
	}

	union, err := OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err == nil {
		_ = union.Close()
		t.Fatal("expected error on space_id mismatch, got nil")
	}
	if union != nil {
		t.Fatalf("expected nil union on error, got %+v", union)
	}
}
