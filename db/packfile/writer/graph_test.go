package writer

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// TestPackGraphKeepsEveryBlock tries to lose, duplicate or misplace a block
// through PackGraph's layout: random graphs with shared children, refs that
// leave the pack, blocks whose refs are unknown, forests and shuffled or
// reversed streams. Every pack must hold exactly the blocks yielded, byte for
// byte, its structure before its leaves, and each structure block's reach must
// cover every structure block below it that an earlier block did not claim.
func TestPackGraphKeepsEveryBlock(t *testing.T) {
	for seed := range uint64(200) {
		rng := rand.New(rand.NewPCG(seed, 7))
		blocks := randomGraph(t, rng, seed%3 == 0)

		// Stream the blocks shuffled, in reverse, or in order.
		stream := slices.Clone(blocks)
		switch seed % 4 {
		case 1:
			rng.Shuffle(len(stream), func(i, j int) { stream[i], stream[j] = stream[j], stream[i] })
		case 2:
			slices.Reverse(stream)
		}
		var buf bytes.Buffer
		if _, err := PackGraph(&buf, sliceIterator(stream)); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		checkGraphPack(t, seed, buf.Bytes(), stream)
	}
}

// TestPackGraphRejectsDuplicates checks that a block yielded twice fails the
// pack instead of one copy replacing the other.
func TestPackGraphRejectsDuplicates(t *testing.T) {
	blocks := randomGraph(t, rand.New(rand.NewPCG(1, 1)), false)
	stream := append(slices.Clone(blocks), blocks[len(blocks)-1])
	if _, err := PackGraph(&bytes.Buffer{}, sliceIterator(stream)); err == nil {
		t.Fatal("expected a duplicate block to fail the pack")
	}
}

// graphBlock is one block of a test graph.
type graphBlock struct {
	h      *hash.Hash
	stored *block.StoredBlock
}

// randomGraph builds 2 to 80 blocks, each referring only to later ones, so
// the graph is acyclic as content addressing makes it. A forest gives each
// block at most one parent; otherwise children are shared freely. Some refs
// name blocks outside the graph, and some blocks have unknown refs.
func randomGraph(t *testing.T, rng *rand.Rand, forest bool) []graphBlock {
	// Hash random data for each block.
	n := 2 + rng.IntN(79)
	blocks := make([]graphBlock, n)
	for i := range blocks {
		data := make([]byte, 1+rng.IntN(300))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		blocks[i] = graphBlock{h: h, stored: &block.StoredBlock{Data: data, RefsSource: block.ListedIf(rng.IntN(10) != 0)}}
	}

	// Link each block with known refs to some later blocks.
	parented := make([]bool, n)
	for i := range blocks {
		if !blocks[i].stored.RefsSource.Recorded() {
			continue
		}
		for range rng.IntN(4) {
			j := i + 1 + rng.IntN(max(1, n-i-1))
			if j >= n || (forest && parented[j]) {
				continue
			}
			parented[j] = true
			blocks[i].stored.Refs = append(blocks[i].stored.Refs, block.NewBlockRef(blocks[j].h))
		}
		if rng.IntN(8) == 0 {
			outside, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{byte(i), 0xfe})
			if err != nil {
				t.Fatal(err)
			}
			blocks[i].stored.Refs = append(blocks[i].stored.Refs, block.NewBlockRef(outside))
		}
	}
	return blocks
}

// sliceIterator yields blocks in order.
func sliceIterator(blocks []graphBlock) BlockIterator {
	i := 0
	return func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(blocks) {
			return nil, nil, nil
		}
		i++
		return blocks[i-1].h, blocks[i-1].stored, nil
	}
}

// checkGraphPack checks a PackGraph pack of stream against the invariants
// TestPackGraphKeepsEveryBlock names.
func checkGraphPack(t *testing.T, seed uint64, data []byte, stream []graphBlock) {
	t.Helper()

	// Every block reads back once with its bytes and refs.
	rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	if rd.Size() != uint64(len(stream)) {
		t.Fatalf("seed %d: pack holds %d blocks, %d yielded", seed, rd.Size(), len(stream))
	}
	entries := make(map[string]*kvfile.IndexEntry, len(stream))
	for _, b := range stream {
		key := packfile.BlockKey(b.h)
		entry, _, err := rd.SearchIndexEntryWithKey(key)
		if err != nil || entry == nil {
			t.Fatalf("seed %d: block %s missing: %v", seed, b.h.MarshalString(), err)
		}
		value, err := rd.GetWithEntry(entry, 0)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		_, got, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if !bytes.Equal(got.Data, b.stored.Data) || got.RefsSource.Recorded() != b.stored.RefsSource.Recorded() || len(got.Refs) != len(b.stored.Refs) {
			t.Fatalf("seed %d: block %s read back changed", seed, b.h.MarshalString())
		}
		for i, ref := range got.Refs {
			if !ref.EqualsRef(b.stored.Refs[i]) {
				t.Fatalf("seed %d: block %s ref %d changed", seed, b.h.MarshalString(), i)
			}
		}
		entries[string(key)] = entry
	}

	// The structure precedes the leaves, and only structure has a reach,
	// ending after the block and within the structure.
	var structureEnd, leafStart uint64 = 0, uint64(len(data))
	for _, b := range stream {
		entry := entries[string(packfile.BlockKey(b.h))]
		end := entry.GetOffset() + entry.GetSize()
		if len(b.stored.Refs) == 0 {
			leafStart = min(leafStart, entry.GetOffset())
			if entry.GetReach() != 0 {
				t.Fatalf("seed %d: leaf with reach %d", seed, entry.GetReach())
			}
			continue
		}
		structureEnd = max(structureEnd, end)
		if entry.GetReach() < end {
			t.Fatalf("seed %d: reach %d ends before its block at %d", seed, entry.GetReach(), end)
		}
	}
	if structureEnd > leafStart {
		t.Fatalf("seed %d: structure ends at %d after the leaves start at %d", seed, structureEnd, leafStart)
	}

	// Each structure block below a structure block lies within its reach,
	// unless a block placed earlier claimed it; roots keep the stream's order.
	byKey := make(map[string]graphBlock, len(stream))
	referenced := make(map[string]bool)
	for _, b := range stream {
		byKey[string(packfile.BlockKey(b.h))] = b
		for _, ref := range b.stored.Refs {
			referenced[string(packfile.BlockKey(ref.GetHash()))] = true
		}
	}
	var lastRoot uint64
	for i, b := range stream {
		key := packfile.BlockKey(b.h)
		entry := entries[string(key)]
		if len(b.stored.Refs) == 0 {
			continue
		}
		if entry.GetReach() > structureEnd {
			t.Fatalf("seed %d: reach %d past the structure's end %d", seed, entry.GetReach(), structureEnd)
		}
		if !referenced[string(key)] {
			if i != 0 && entry.GetOffset() < lastRoot {
				t.Fatalf("seed %d: root placed out of stream order", seed)
			}
			lastRoot = entry.GetOffset()
		}
		for _, below := range descendants(byKey, b) {
			d := entries[below]
			if len(byKey[below].stored.Refs) == 0 {
				continue
			}
			inside := d.GetOffset() >= entry.GetOffset() && d.GetOffset()+d.GetSize() <= entry.GetReach()
			if !inside && d.GetOffset() > entry.GetOffset() {
				t.Fatalf("seed %d: a structure block below %s lies past its reach", seed, b.h.MarshalString())
			}
		}
	}
}

// descendants returns the keys of the blocks in byKey below b.
func descendants(byKey map[string]graphBlock, b graphBlock) []string {
	var out []string
	seen := make(map[string]bool)
	stack := slices.Clone(b.stored.Refs)
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		key := string(packfile.BlockKey(ref.GetHash()))
		child, ok := byKey[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
		stack = append(stack, child.stored.Refs...)
	}
	return out
}
