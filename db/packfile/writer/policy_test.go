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
	if policy.MaxBlocksPerPack != 4096 || policy.BloomExpectedBlocks != 4096 {
		t.Fatalf(
			"block ceiling/expected bloom blocks = %d/%d, want 4096/4096",
			policy.MaxBlocksPerPack,
			policy.BloomExpectedBlocks,
		)
	}
	if policy.BloomFalsePositive != 0.008 {
		t.Fatalf("BloomFalsePositive = %f, want 0.008", policy.BloomFalsePositive)
	}
	if !policy.RequireBloomFilter || !policy.RequireBlockCount || !policy.RequireCreatedAt {
		t.Fatalf("metadata requirements not all enabled: %+v", policy)
	}

	bf := policy.NewBloomFilter()
	if bf.Cap() == 0 || bf.K() == 0 {
		t.Fatalf("invalid bloom parameters m=%d k=%d", bf.Cap(), bf.K())
	}
	fp := bloom.EstimateFalsePositiveRate(
		bf.Cap(),
		bf.K(),
		uint(policy.BloomExpectedBlocks),
	)
	if fp > policy.BloomFalsePositive*1.01 {
		t.Fatalf("estimated FPR = %f, want near %f", fp, policy.BloomFalsePositive)
	}
}
