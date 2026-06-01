//go:build js

package blockshard

import "github.com/s4wave/spacewave/db/volume/js/opfs/segment"

const (
	// DefaultMaxSegmentDataBytes targets one current large blob chunk before
	// indexes, bloom filters, and headers are added. Single larger entries still
	// publish as one segment. Browser TinyGo builds duplicate SSTable data while
	// producing the final segment, so the default keeps normal Spacewave upload
	// chunks below multi-megabyte Go heap growth in the blockshard writer.
	DefaultMaxSegmentDataBytes = 768 << 10
	// DefaultMaxEntryValueBytes rejects impossible-large single entries before
	// the segment writer duplicates them into an SSTable buffer. Normal UnixFS
	// blob chunks are below this bound; larger user files must arrive as chunked
	// blobs rather than one raw Blockshard value.
	DefaultMaxEntryValueBytes = 2 << 20
)

// Settings configures the block shard engine.
type Settings struct {
	ShardCount          int
	BloomFPR            float64
	CompactionTrigger   int
	AsyncIO             bool
	MaxSegmentDataBytes int
	MaxEntryValueBytes  int
}

// DefaultSettings returns the default block shard settings.
func DefaultSettings() *Settings {
	return &Settings{
		ShardCount:          DefaultShardCount,
		BloomFPR:            segment.DefaultBloomFPR,
		CompactionTrigger:   DefaultL0Trigger,
		MaxSegmentDataBytes: DefaultMaxSegmentDataBytes,
		MaxEntryValueBytes:  DefaultMaxEntryValueBytes,
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
	if out.MaxEntryValueBytes < 1 {
		out.MaxEntryValueBytes = DefaultMaxEntryValueBytes
	}
	return &out
}
