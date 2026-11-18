//go:build !tinygo

package block_store_s3

import "context"

// ColdPackRangeReader reads immutable object ranges. ColdPackStore has no
// capability to list, upload, or delete either hot or cold objects.
type ColdPackRangeReader interface {
	// GetColdPackRange reads length bytes at off, rejecting a different object
	// size. It also returns an optional SHA-256 of the entire object.
	GetColdPackRange(ctx context.Context, bucket, key string, off int64, length int, size uint64) ([]byte, []byte, error)
}

// ColdPackOptions describes a mounted immutable pack set and its cache bounds.
type ColdPackOptions struct {
	// Prefix is the dedicated bucket prefix, outside the main store's object directories.
	Prefix string
	// ManifestFile is the local manifest produced by BuildColdPackManifest.
	ManifestFile string
	// PromotedLog is a dedicated, exclusively owned local log path. Empty disables promotion.
	PromotedLog string
	// PromoteCap bounds the log to this many 32-byte records. Zero disables promotion.
	PromoteCap int
	// TailCacheBytes bounds resident raw index tails. Zero uses 64 MiB.
	TailCacheBytes int
	// NegativeCacheKeys bounds remembered exact misses. Zero uses 65,536 full digests.
	NegativeCacheKeys int
}
