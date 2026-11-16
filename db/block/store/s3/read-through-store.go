//go:build !tinygo

package block_store_s3

import (
	"context"
	"strings"
)

// ReadThroughStore overrides only pack range reads. All writes, deletions,
// listings, entry reads and whole pack reads retain the direct origin path.
type ReadThroughStore struct {
	// ObjectStore is the direct origin, including every mutating operation.
	ObjectStore
	// Cache is the optional range cache.
	Cache *HTTPRangeCache
	// Bucket scopes cached reads to this bucket.
	Bucket string
	// PackPrefix scopes cached reads to one block store's packs.
	PackPrefix string
}

// GetObjectRange reads a pack range through Cache with direct fallback.
func (s *ReadThroughStore) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if bucket == s.Bucket && strings.HasPrefix(key, s.PackPrefix) && !strings.Contains(strings.TrimPrefix(key, s.PackPrefix), "/") {
		return s.Cache.Read(ctx, s.ObjectStore, bucket, key, off, length)
	}
	return s.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

var _ ObjectStore = (*ReadThroughStore)(nil)
