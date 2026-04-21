//go:build js

package blockshard

import "github.com/s4wave/spacewave/db/volume/js/opfs/segment"

// Settings configures the block shard engine.
type Settings struct {
	ShardCount        int
	BloomFPR          float64
	CompactionTrigger int
	AsyncIO           bool
}

// DefaultSettings returns the benchmark-selected default block shard settings.
func DefaultSettings() *Settings {
	return &Settings{
		ShardCount:        DefaultShardCount,
		BloomFPR:          segment.DefaultBloomFPR,
		CompactionTrigger: DefaultL0Trigger,
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
	return &out
}
