package provider_local

import (
	"sync/atomic"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// s3ReadCacheBinding scopes a process-local, secret-bearing optional cache.
type s3ReadCacheBinding struct {
	// endpoint is the S3 origin to match.
	endpoint string
	// bucket is the bucket to match.
	bucket string
	// blockStoreID is the single block store to match.
	blockStoreID string
	// cache reads bounded ranges without changing stored account settings.
	cache *block_store_s3.HTTPRangeCache
}

// s3ReadCache holds the optional cache configured before accounts start.
var s3ReadCache atomic.Pointer[s3ReadCacheBinding]

// SetS3ReadCache installs a private range cache for one origin, bucket and
// block store. Call before opening accounts. A nil cache disables it. The
// token remains process-local and is never persisted in account settings.
func SetS3ReadCache(endpoint, bucket, blockStoreID string, cache *block_store_s3.HTTPRangeCache) {
	if cache == nil {
		s3ReadCache.Store(nil)
		return
	}
	s3ReadCache.Store(&s3ReadCacheBinding{endpoint: endpoint, bucket: bucket, blockStoreID: blockStoreID, cache: cache})
}
