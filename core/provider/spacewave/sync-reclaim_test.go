package provider_spacewave

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// reclaimTestBlock is one stored block with its ref.
type reclaimTestBlock struct {
	ref    *block.BlockRef
	stored *block.StoredBlock
}

// newReclaimTestBlock stores data with refs.
func newReclaimTestBlock(t *testing.T, data []byte, refs ...*block.BlockRef) reclaimTestBlock {
	t.Helper()
	h, err := hash.Sum(hash.RecommendedHashType, data)
	if err != nil {
		t.Fatal(err)
	}
	return reclaimTestBlock{ref: block.NewBlockRef(h), stored: &block.StoredBlock{Data: data, Refs: refs, RefsKnown: true}}
}

// addReclaimTestPack commits blocks as pack id at sequence seq.
func addReclaimTestPack(t *testing.T, s *syncController, cloud *compactTestCloud, id string, seq uint64, blocks ...reclaimTestBlock) *packfile.PackfileEntry {
	// Pack the blocks in order and serve the pack from the cloud.
	t.Helper()
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.ref.GetHash(), b.stored, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cloud.packs[id] = buf.Bytes()

	// Commit the entry to the manifest and the lower store.
	entry := &packfile.PackfileEntry{
		Id:                 id,
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		Sequence:           seq,
	}
	setReclaimTestEntries(t, s, entry)
	return entry
}

// setReclaimTestEntries applies catalog entries as a pull would.
func setReclaimTestEntries(t *testing.T, s *syncController, entries ...*packfile.PackfileEntry) {
	t.Helper()
	if err := s.applyManifestDelta(t.Context(), entries, nil, 0); err != nil {
		t.Fatal(err)
	}
	s.lower.UpdateManifest(s.mfst.GetEntries())
}

// TestSyncControllerReclaimTrashesAndRetires verifies a pass trashes the
// mostly dead packs, skips a pass that would free nothing, and later retires
// due trash after rescuing a live block no other pack holds.
func TestSyncControllerReclaimTrashesAndRetires(t *testing.T) {
	// Build a live root over a leaf and two dead blocks.
	ctx := t.Context()
	leaf := newReclaimTestBlock(t, []byte("live leaf"))
	root := newReclaimTestBlock(t, []byte("live root"), leaf.ref)
	deadA := newReclaimTestBlock(t, bytes.Repeat([]byte("a"), 4096))
	deadB := newReclaimTestBlock(t, bytes.Repeat([]byte("b"), 4096))

	// Commit a mostly dead pack with the leaf, a dead pack and a live pack.
	cloud := &compactTestCloud{packs: make(map[string][]byte)}
	s := newCompactTestController(t, cloud, nil)
	mixed := addReclaimTestPack(t, s, cloud, "pack-mixed", 1, leaf, deadA)
	dead := addReclaimTestPack(t, s, cloud, "pack-dead", 2, deadB)
	addReclaimTestPack(t, s, cloud, "pack-live", 3, root)

	// Fence on the root, counting the passes that reach the fence.
	var fences int
	fence := func(context.Context, bool) ([]*block.BlockRef, error) {
		fences++
		return []*block.BlockRef{root.ref}, nil
	}

	// The first pass trashes both dead-heavy packs.
	next, err := s.ReclaimStorage(ctx, fence)
	if err != nil {
		t.Fatal(err)
	}
	if len(cloud.trash) != 1 || !slices.Equal(cloud.trash[0].GetTrashPackIds(), []string{dead.GetId(), mixed.GetId()}) {
		t.Fatalf("trash requests = %v", cloud.trash)
	}
	if want := time.Now().Add(reclaimInterval); next.Before(want.Add(-time.Minute)) || next.After(want) {
		t.Fatalf("next pass at %v, want about %v", next, want)
	}

	// A pass with no growth and no due trash does not fence.
	if _, err := s.ReclaimStorage(ctx, fence); err != nil || fences != 1 {
		t.Fatalf("idle pass: fences=%d err=%v", fences, err)
	}

	// Once the trash is old enough, a pass rescues the live leaf and retires
	// both trash packs.
	trashedAt := timestamppb.New(time.Now().Add(-packfile.TrashAge - time.Minute))
	mixed, dead = mixed.CloneVT(), dead.CloneVT()
	mixed.Sequence, mixed.TrashedAt = 4, trashedAt
	dead.Sequence, dead.TrashedAt = 5, trashedAt
	setReclaimTestEntries(t, s, mixed, dead)
	if _, err := s.ReclaimStorage(ctx, fence); err != nil {
		t.Fatal(err)
	}
	if len(cloud.trash) != 2 || !slices.Equal(cloud.trash[1].GetRetirePackIds(), []string{dead.GetId(), mixed.GetId()}) {
		t.Fatalf("retire requests = %v", cloud.trash)
	}
	if len(cloud.pushes) != 1 || cloud.pushes[0] != "" {
		t.Fatalf("rescue pushes = %q", cloud.pushes)
	}

	// The rescue pack holds exactly the live leaf.
	var rescued [][]byte
	for id, data := range cloud.packs {
		if id == mixed.GetId() || id == dead.GetId() || id == "pack-live" {
			continue
		}
		rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		_ = rd.ScanPrefixKeys(nil, func(key []byte) error {
			rescued = append(rescued, bytes.Clone(key))
			return nil
		})
	}
	if len(rescued) != 1 || !bytes.Equal(rescued[0], packfile.BlockKey(leaf.ref.GetHash())) {
		t.Fatalf("rescued keys = %x", rescued)
	}
}

// TestSyncControllerReclaimMissingBlock verifies a walk that cannot read a
// live block ends the pass with no change.
func TestSyncControllerReclaimMissingBlock(t *testing.T) {
	// Commit a dead pack and fence on a root no store holds.
	cloud := &compactTestCloud{packs: make(map[string][]byte)}
	s := newCompactTestController(t, cloud, nil)
	addReclaimTestPack(t, s, cloud, "pack-dead", 1, newReclaimTestBlock(t, []byte("dead")))
	missing := newReclaimTestBlock(t, []byte("missing"))
	fence := func(context.Context, bool) ([]*block.BlockRef, error) {
		return []*block.BlockRef{missing.ref}, nil
	}

	// The pass fails before judging any pack.
	if _, err := s.ReclaimStorage(t.Context(), fence); err == nil || len(cloud.trash) != 0 {
		t.Fatalf("missing block: err=%v trash=%v", err, cloud.trash)
	}
}

// TestPlanCompactionSkipsTrash verifies a merge never takes a trash pack.
func TestPlanCompactionSkipsTrash(t *testing.T) {
	// Build enough small packs, the oldest of them trash.
	entries := make([]*packfile.PackfileEntry, compactMinPacks)
	for i := range entries {
		entries[i] = &packfile.PackfileEntry{Id: string(rune('a' + i)), Sequence: uint64(i + 1), SizeBytes: 1} //nolint:gosec // small test index.
	}
	entries[0].TrashedAt = timestamppb.Now()

	// One trash pack leaves too few to merge.
	if inputs := planCompaction(entries); inputs != nil {
		t.Fatalf("planned %d inputs with a trash pack", len(inputs))
	}
}
