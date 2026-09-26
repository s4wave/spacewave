package writer

import (
	"bytes"
	"io"
	"strconv"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// TestPackBlocks verifies that PackBlocks writes a valid kvfile whose values
// carry each block's bytes and refs, the bloom filter contains all packed
// hashes, and rejects unknown hashes.
func TestPackBlocks(t *testing.T) {
	// Generate test blocks, each referencing the one before it.
	type testBlock struct {
		hash *hash.Hash
		data []byte
		refs []*block.BlockRef
	}
	var blocks []testBlock
	for i := range 10 {
		data := []byte("block-data-" + string(rune('A'+i)))
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		refs := []*block.BlockRef{}
		if i != 0 {
			refs = append(refs, block.NewBlockRef(blocks[i-1].hash))
		}
		blocks = append(blocks, testBlock{hash: h, data: data, refs: refs})
	}

	// Pack blocks.
	var buf bytes.Buffer
	idx := 0
	result, err := PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.hash, &block.StoredBlock{Data: b.data, Refs: b.refs, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.BlockCount != uint64(len(blocks)) {
		t.Fatalf("expected block count %d, got %d", len(blocks), result.BlockCount)
	}
	if result.BytesWritten == 0 {
		t.Fatal("expected non-zero bytes written")
	}
	if len(result.BloomFilter) == 0 {
		t.Fatal("expected non-empty bloom filter")
	}

	// Verify kvfile is readable and contains all blocks with their refs.
	rd := bytes.NewReader(buf.Bytes())
	reader, err := kvfile.BuildReader(rd, uint64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if reader.Size() != uint64(len(blocks)) {
		t.Fatalf("expected %d entries, got %d", len(blocks), reader.Size())
	}

	for _, b := range blocks {
		key := packfile.BlockKey(b.hash)
		value, found, err := reader.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("block %s not found in kvfile", b.hash.MarshalString())
		}
		_, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored.GetData(), b.data) {
			t.Fatalf("block data mismatch for %s", b.hash.MarshalString())
		}
		if len(stored.GetRefs()) != len(b.refs) {
			t.Fatalf("block %s has %d refs, want %d", b.hash.MarshalString(), len(stored.GetRefs()), len(b.refs))
		}
		for i, ref := range b.refs {
			if !stored.GetRefs()[i].EqualsRef(ref) {
				t.Fatalf("block %s ref %d mismatch", b.hash.MarshalString(), i)
			}
		}
	}

	// Verify bloom filter contains all hashes.
	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(result.BloomFilter); err != nil {
		t.Fatal(err)
	}
	bf := pbf.ToBloomFilter()
	if bf == nil {
		t.Fatal("bloom filter deserialized to nil")
	}
	policyBloom := DefaultPolicy().NewBloomFilter(uint64(len(blocks)))
	if bf.Cap() != policyBloom.Cap() {
		t.Fatalf("bloom cap = %d, want %d", bf.Cap(), policyBloom.Cap())
	}
	if bf.K() != policyBloom.K() {
		t.Fatalf("bloom k = %d, want %d", bf.K(), policyBloom.K())
	}

	for _, b := range blocks {
		key := packfile.BlockKey(b.hash)
		if !bf.Test(key) {
			t.Fatalf("bloom filter should contain %s", b.hash.MarshalString())
		}
	}

	// Verify bloom rejects an unknown hash.
	unknownData := []byte("definitely-not-in-the-packfile")
	unknownHash, err := hash.Sum(hash.HashType_HashType_SHA256, unknownData)
	if err != nil {
		t.Fatal(err)
	}
	unknownKey := packfile.BlockKey(unknownHash)
	// Bloom may false-positive, but with 10 items and 1% FPR it is unlikely.
	// We just log it rather than fail.
	if bf.Test(unknownKey) {
		t.Log("bloom false positive for unknown hash (acceptable)")
	}

	// Verify empty pack.
	var emptyBuf bytes.Buffer
	emptyResult, err := PackBlocks(&emptyBuf, func() (*hash.Hash, *block.StoredBlock, error) {
		return nil, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if emptyResult.BlockCount != 0 {
		t.Fatalf("expected 0 blocks, got %d", emptyResult.BlockCount)
	}

	// Verify error propagation.
	_, err = PackBlocks(io.Discard, func() (*hash.Hash, *block.StoredBlock, error) {
		return nil, nil, io.ErrUnexpectedEOF
	})
	if err == nil {
		t.Fatal("expected error from iterator")
	}

	// Verify a block without known refs is rejected.
	_, err = PackBlocks(io.Discard, func() (*hash.Hash, *block.StoredBlock, error) {
		return blocks[0].hash, &block.StoredBlock{Data: blocks[0].data}, nil
	})
	if !errors.Is(err, block.ErrRefsUnknown) {
		t.Fatalf("pack byte-only block err = %v, want ErrRefsUnknown", err)
	}
}

// TestPackBlocksPolicyFalsePositiveRate checks the observed false-positive
// rate of packed filters from a small pack up to the block ceiling.
func TestPackBlocksPolicyFalsePositiveRate(t *testing.T) {
	policy := DefaultPolicy()
	for _, blockCount := range []int{64, int(policy.MaxBlocksPerPack)} {
		t.Run(strconv.Itoa(blockCount), func(t *testing.T) {
			testPackBlocksFalsePositiveRate(t, policy, blockCount)
		})
	}
}

func testPackBlocksFalsePositiveRate(t *testing.T, policy Policy, blockCount int) {
	blocks := make([]*hash.Hash, 0, blockCount)

	var buf bytes.Buffer
	idx := 0
	result, err := PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= blockCount {
			return nil, nil, nil
		}
		data := []byte("policy false-positive block " + strconv.Itoa(idx))
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, h)
		idx++
		return h, &block.StoredBlock{Data: data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatalf("pack blocks: %v", err)
	}
	if result.BlockCount != uint64(blockCount) {
		t.Fatalf("BlockCount = %d, want %d", result.BlockCount, blockCount)
	}

	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(result.BloomFilter); err != nil {
		t.Fatal(err)
	}
	bf := pbf.ToBloomFilter()
	if bf == nil {
		t.Fatal("bloom filter deserialized to nil")
	}

	for _, h := range blocks {
		if !bf.Test(packfile.BlockKey(h)) {
			t.Fatalf("bloom filter should contain %s", h.MarshalString())
		}
	}

	samples := 10000
	falsePositives := 0
	for i := range samples {
		data := []byte("policy absent block " + strconv.Itoa(i))
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatalf("sum absent block hash: %v", err)
		}
		if bf.Test(packfile.BlockKey(h)) {
			falsePositives++
		}
	}
	rate := float64(falsePositives) / float64(samples)
	if rate > policy.BloomFalsePositive*3 {
		t.Fatalf(
			"observed false-positive rate = %.4f (%d/%d), want under %.4f",
			rate,
			falsePositives,
			samples,
			policy.BloomFalsePositive*3,
		)
	}
}
