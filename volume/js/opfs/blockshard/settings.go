//go:build js

package blockshard

import (
	"time"

	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
)

// Settings configures the block shard engine.
type Settings struct {
	ShardCount        int
	BloomFPR          float64
	FlushThreshold    int
	FlushMaxAge       time.Duration
	CompactionTrigger int
}

// DefaultSettings returns the benchmark-selected default block shard settings.
func DefaultSettings() *Settings {
	return &Settings{
		ShardCount:        DefaultShardCount,
		BloomFPR:          segment.DefaultBloomFPR,
		FlushThreshold:    DefaultFlushThreshold,
		FlushMaxAge:       DefaultFlushMaxAge,
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
	if out.FlushThreshold < 1 {
		out.FlushThreshold = DefaultFlushThreshold
	}
	if out.FlushMaxAge <= 0 {
		out.FlushMaxAge = DefaultFlushMaxAge
	}
	if out.CompactionTrigger < 2 {
		out.CompactionTrigger = DefaultL0Trigger
	}
	return &out
}
