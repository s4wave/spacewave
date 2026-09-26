package writer

import "github.com/s4wave/spacewave/db/block/bloom"

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
	MaxPackBytes       int64
	MaxBlocksPerPack   uint64
	BloomFalsePositive float64
	RequireBloomFilter bool
	RequireBlockCount  bool
	RequireCreatedAt   bool
}

// DefaultPolicy returns the shared Spacewave pack construction policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxPackBytes:       DefaultMaxPackBytes,
		MaxBlocksPerPack:   DefaultMaxBlocksPerPack,
		BloomFalsePositive: DefaultBloomFalsePositiveRate,
		RequireBloomFilter: true,
		RequireBlockCount:  true,
		RequireCreatedAt:   true,
	}
}

// NewBloomFilter creates an empty bloom filter sized for a pack of
// blockCount blocks at the policy false-positive rate.
func (p Policy) NewBloomFilter(blockCount uint64) *bloom.Filter {
	fp := p.BloomFalsePositive
	if fp <= 0 {
		fp = DefaultBloomFalsePositiveRate
	}
	return bloom.NewFilter(uint(blockCount), fp)
}
