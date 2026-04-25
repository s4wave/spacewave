package writer

import bbloom "github.com/bits-and-blooms/bloom/v3"

const (
	// DefaultMaxPackBytes is the byte ceiling for a single uploaded packfile.
	DefaultMaxPackBytes int64 = 63 * 1024 * 1024
	// DefaultMaxBlocksPerPack is the block-count ceiling for one packfile.
	DefaultMaxBlocksPerPack uint64 = 4096
	// DefaultBloomFalsePositiveRate is the target pack bloom false-positive rate.
	DefaultBloomFalsePositiveRate = 0.008
)

// Policy describes pack construction limits and required metadata.
type Policy struct {
	MaxPackBytes        int64
	MaxBlocksPerPack    uint64
	BloomExpectedBlocks uint64
	BloomFalsePositive  float64
	RequireBloomFilter  bool
	RequireBlockCount   bool
	RequireCreatedAt    bool
}

// DefaultPolicy returns the shared Spacewave pack construction policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxPackBytes:        DefaultMaxPackBytes,
		MaxBlocksPerPack:    DefaultMaxBlocksPerPack,
		BloomExpectedBlocks: DefaultMaxBlocksPerPack,
		BloomFalsePositive:  DefaultBloomFalsePositiveRate,
		RequireBloomFilter:  true,
		RequireBlockCount:   true,
		RequireCreatedAt:    true,
	}
}

// NewBloomFilter creates an empty bloom filter for the policy.
func (p Policy) NewBloomFilter() *bbloom.BloomFilter {
	n := p.BloomExpectedBlocks
	if n == 0 {
		n = DefaultMaxBlocksPerPack
	}
	fp := p.BloomFalsePositive
	if fp <= 0 {
		fp = DefaultBloomFalsePositiveRate
	}
	return bbloom.NewWithEstimates(uint(n), fp)
}
