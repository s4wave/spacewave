//go:build js

package blockshard

import "github.com/s4wave/spacewave/db/volume/js/opfs/segment"

const (
	// DefaultMaxSegmentDataBytes bounds one SSTable data block before indexes,
	// bloom filters, and headers are added. Browser TinyGo builds duplicate the
	// data block while producing the final SSTable, so large uploads must publish
	// multiple small immutable segments instead of one large segment buffer.
	DefaultMaxSegmentDataBytes = 2 << 20
)

// Settings configures the block shard engine.
type Settings struct {
	ShardCount          int
	BloomFPR            float64
	CompactionTrigger   int
	AsyncIO             bool
	MaxSegmentDataBytes int
}

// DefaultSettings returns the benchmark-selected default block shard settings.
func DefaultSettings() *Settings {
	return &Settings{
		ShardCount:          DefaultShardCount,
		BloomFPR:            segment.DefaultBloomFPR,
		CompactionTrigger:   DefaultL0Trigger,
		MaxSegmentDataBytes: DefaultMaxSegmentDataBytes,
	}
}

func normalizeSettings(s *Settings) *Settings {
	if s == nil {
		return DefaultSettings()
	}
	out := *s
	if out.ShardCount < 1 {
		out.ShardCount = DefaultShardCount
	}
	if out.BloomFPR <= 0 || out.BloomFPR >= 1 {
		out.BloomFPR = segment.DefaultBloomFPR
	}
	if out.CompactionTrigger < 2 {
		out.CompactionTrigger = DefaultL0Trigger
	}
	if out.MaxSegmentDataBytes < 1 {
		out.MaxSegmentDataBytes = DefaultMaxSegmentDataBytes
	}
	return &out
}
