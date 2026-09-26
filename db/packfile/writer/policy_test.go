package writer

import (
	"testing"

	"github.com/s4wave/spacewave/db/block/bloom"
)

func TestDefaultPolicy(t *testing.T) {
	policy := DefaultPolicy()
	if policy.MaxPackBytes != 63*1024*1024 {
		t.Fatalf("MaxPackBytes = %d, want 63 MiB", policy.MaxPackBytes)
	}
	if policy.MaxBlocksPerPack != 4096 {
		t.Fatalf("MaxBlocksPerPack = %d, want 4096", policy.MaxBlocksPerPack)
	}
	if policy.BloomFalsePositive != 0.008 {
		t.Fatalf("BloomFalsePositive = %f, want 0.008", policy.BloomFalsePositive)
	}
	if !policy.RequireBloomFilter || !policy.RequireBlockCount || !policy.RequireCreatedAt {
		t.Fatalf("metadata requirements not all enabled: %+v", policy)
	}
}

// TestPolicyBloomFilterSizedToBlockCount checks that a pack's filter grows
// with its block count and holds the policy false-positive rate at each size.
func TestPolicyBloomFilterSizedToBlockCount(t *testing.T) {
	policy := DefaultPolicy()
	var prevCap uint
	for _, n := range []uint64{1, 64, 1024, 4096, 20000} {
		bf := policy.NewBloomFilter(n)
		if bf.Cap() == 0 || bf.K() == 0 {
			t.Fatalf("n=%d: invalid bloom parameters m=%d k=%d", n, bf.Cap(), bf.K())
		}
		if bf.Cap() <= prevCap {
			t.Fatalf("n=%d: m=%d, want more than %d", n, bf.Cap(), prevCap)
		}
		prevCap = bf.Cap()
		fp := bloom.EstimateFalsePositiveRate(bf.Cap(), bf.K(), uint(n))
		if fp > policy.BloomFalsePositive*1.01 {
			t.Fatalf("n=%d: estimated FPR = %f, want near %f", n, fp, policy.BloomFalsePositive)
		}
	}
}
