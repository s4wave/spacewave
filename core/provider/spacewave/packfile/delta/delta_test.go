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
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"

	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// blockSpec describes a block that will be packed into a test kvfile.
type blockSpec struct {
	key  string
	data []byte
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
	// Mark the helper.
	t.Helper()

	// Build deterministic block specs with hashed keys.
	specs = make([]blockSpec, 0, n)
	for i := range n {
		body := make([]byte, sizePerBlock)
		for j := range body {
			body[j] = byte((i*13 + j) & 0xff)
		}
		// Stamp the block body with its index or prefix.
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

	// Pack the specs into a kvfile buffer.
	var buf bytes.Buffer
	idx := 0
	if _, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(specs) {
			return nil, nil, nil
		}
		s := specs[idx]
		idx++
		return blockRefFromKey(t, s.key).GetHash(), &block.StoredBlock{Data: s.data, RefsKnown: true}, nil
	}); err != nil {
		t.Fatalf("pack blocks: %v", err)
	}

	// Build the kvfile reader over the packed bytes.
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

func newMirrorStore(t *testing.T, ctx context.Context, specs ...blockSpec) block.StoreOps {
	t.Helper()
	// Build the in-memory mirror store and fill it with the specs.
	store := block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.HashType_HashType_SHA256,
		false,
	)
	// Put each spec block and assert its ref key.
	for _, spec := range specs {
		ref, _, err := store.PutBlock(ctx, spec.data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := ref.GetHash().MarshalString(); got != spec.key {
			t.Fatalf("mirror ref = %s, want %s", got, spec.key)
		}
	}
	return store
}

func packPhysicalKeys(t *testing.T, body []byte) []string {
	// Mark the helper.
	t.Helper()

	// Read the pack and collect its index entries.
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

	// Sort entries by physical offset and decode their keys.
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

// TestDiffBlockStoresEmpty verifies that when every source block already lives
// in the mirror, DiffBlockStores yields an exhausted iterator and
// EmitDeltaChunks emits nothing (empty-diff no-op).
func TestDiffBlockStoresEmpty(t *testing.T) {
	// Run the empty-diff case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Build a source kvfile fully mirrored in the store.
	specs, reader := buildTestKvfile(t, "empty", 4, 64)
	mirror := newMirrorStore(t, ctx, specs...)

	// Diff the source against the mirror.
	iter, err := DiffBlockStores(ctx, reader, mirror)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	// Emit chunks and assert nothing was emitted.
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
	// Run the single-chunk case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Mirror only the first two specs; the remaining four should be packed.
	specs, reader := buildTestKvfile(t, "single", 6, 128)
	mirror := newMirrorStore(t, ctx, specs[0], specs[1])
	expectedCount := uint64(len(specs) - 2)

	// Diff the source against the partial mirror.
	iter, err := DiffBlockStores(ctx, reader, mirror)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	// Emit chunks and collect their bytes.
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

	// Assert the single entry's metadata fields.
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
		key := packfile.BlockKey(blockRefFromKey(t, s.key).GetHash())
		value, found, err := reader2.Get(key)
		if err != nil {
			t.Fatalf("Get %s: %v", s.key, err)
		}
		if !found {
			t.Fatalf("block %s missing from chunk", s.key)
		}
		_, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			t.Fatalf("decode %s: %v", s.key, err)
		}
		if !bytes.Equal(stored.GetData(), s.data) {
			t.Fatalf("block %s data mismatch", s.key)
		}
	}
}

func TestDiffBlockStoresWithRefGraphOrdersPhysicalPack(t *testing.T) {
	// Run the graph-ordering case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Build a kvfile with four blocks and reference them in a graph.
	specs, reader := buildTestKvfile(t, "graph", 4, 128)
	stray := blockRefFromKey(t, specs[0].key)
	rootA := blockRefFromKey(t, specs[1].key)
	rootB := blockRefFromKey(t, specs[2].key)
	childA := blockRefFromKey(t, specs[3].key)

	// Add ownership and child edges to the test graph.
	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("object-b"), block_gc.BlockIRI(rootB))
	graph.add(block_gc.ObjectIRI("object-a"), block_gc.BlockIRI(rootA))
	graph.add(block_gc.BlockIRI(rootA), block_gc.BlockIRI(childA))

	// Diff with the graph and collect the emitted chunk.
	iter, err := DiffBlockStoresWithRefGraph(ctx, reader, nil, graph)
	if err != nil {
		t.Fatalf("DiffBlockStoresWithRefGraph: %v", err)
	}

	// Emit the ordered chunk.
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

	// Assert the physical pack order matches the graph order.
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
	// Run the multi-chunk case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Build blocks larger than half the chunk limit.
	const nBlocks = 3
	const blockSize = 1024
	specs, reader := buildTestKvfile(t, "multi", nBlocks, blockSize)

	// Diff the source without a mirror.
	iter, err := DiffBlockStores(ctx, reader, nil)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	// Force one block per chunk by choosing maxBytes below 2*blockSize.
	maxBytes := int64(blockSize + 256)

	// Emit chunks with the tight byte limit and collect sizes.
	var emitted []*packfile.PackfileEntry
	var sizes []uint64
	emitted, err = EmitDeltaChunks(ctx, "test-resource", iter, maxBytes, func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error {
		sizes = append(sizes, entry.GetSizeBytes())
		return nil
	})
	if err != nil {
		t.Fatalf("EmitDeltaChunks: %v", err)
	}

	// Assert each chunk holds exactly one block within the limit.
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

	// Assert the pack ids are unique and non-empty.
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
	// Run the block-count ceiling case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Build one more block than the pack ceiling.
	nBlocks := int(writer.DefaultMaxBlocksPerPack) + 1
	_, reader := buildTestKvfile(t, "count", nBlocks, 32)

	// Diff the source without a mirror.
	iter, err := DiffBlockStores(ctx, reader, nil)
	if err != nil {
		t.Fatalf("DiffBlockStores: %v", err)
	}

	// Emit chunks and assert the split respects the ceiling.
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
	// Run the absent-mirror case in parallel.
	t.Parallel()
	ctx := context.Background()

	// No {mirrorDir}/{spaceID}/ subdir; should degenerate cleanly.
	mirrorDir := t.TempDir()
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
	// Run the raw-key pack case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Hash a block body and pack it into a raw pack.
	body := []byte("mirror raw pack block")
	h, err := hash.Sum(hash.HashType_HashType_SHA256, body)
	if err != nil {
		t.Fatalf("hash.Sum: %v", err)
	}
	emitted := false
	var pack bytes.Buffer
	if _, err := writer.PackBlocks(&pack, func() (*hash.Hash, *block.StoredBlock, error) {
		if emitted {
			return nil, nil, nil
		}
		emitted = true
		return h, &block.StoredBlock{Data: body, RefsKnown: true}, nil
	}); err != nil {
		t.Fatalf("PackBlocks: %v", err)
	}

	// Write the pack into a mirror directory layout.
	mirrorDir := t.TempDir()
	spaceID := "01test000000000000rawkeys"
	packDir := filepath.Join(mirrorDir, spaceID, "packs", "01")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "01PACK.kvf"), pack.Bytes(), 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	// Open the union and read the block back through it.
	union, err := OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err != nil {
		t.Fatalf("OpenMirrorUnion: %v", err)
	}
	defer union.Close()

	// Read the block back through the union.
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
	// Run the space-id mismatch case in parallel.
	t.Parallel()
	ctx := context.Background()

	// Create a mirror dir with a mismatching root pointer.
	mirrorDir := t.TempDir()
	spaceID := "01test00000000000000abcd00"
	otherID := "01test00000000000000wxyz00"
	spaceDir := filepath.Join(mirrorDir, spaceID)
	if err := os.MkdirAll(filepath.Join(spaceDir, "packs"), 0o755); err != nil {
		t.Fatalf("mkdir packs: %v", err)
	}

	// Marshal and write the mismatched root pointer.
	ptr := &alpha_cdn.CdnRootPointer{SpaceId: otherID}
	body, err := ptr.MarshalVT()
	if err != nil {
		t.Fatalf("marshal ptr: %v", err)
	}
	encoded := packedmsg.EncodePackedMessage(body)
	if err := os.WriteFile(filepath.Join(spaceDir, "root.packedmsg"), []byte(encoded), 0o644); err != nil {
		t.Fatalf("write root.packedmsg: %v", err)
	}

	// Open the union and assert the space-id mismatch fails.
	union, err := OpenMirrorUnion(ctx, nil, mirrorDir, spaceID)
	if err == nil {
		_ = union.Close()
		t.Fatal("expected error on space_id mismatch, got nil")
	}
	if union != nil {
		t.Fatalf("expected nil union on error, got %+v", union)
	}
}
