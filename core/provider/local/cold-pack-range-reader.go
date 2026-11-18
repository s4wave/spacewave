package provider_local

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// coldPackRangeReader borrows the hot backend's S3 client for cold range reads
// without allowing the cold manifest to follow a change of placement.
type coldPackRangeReader struct {
	// tracker publishes the current backend client after it opens.
	tracker *bstoreTracker
	// backend is the mount's S3 location and credentials identity.
	backend *account_settings.StorageBackend
}

// GetObjectRange reads a cold extent using the matching open backend. Unlike
// the main read path, it never waits for an unavailable backend to open.
func (r *coldPackRangeReader) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// A missing or changed placement cannot read this manifest's objects.
	opened := r.tracker.remote.Load()
	if opened == nil || opened.s3 == nil || !opened.backend.EqualVT(r.backend) {
		return nil, block.ErrUnavailable
	}
	return opened.s3.GetObjectRange(ctx, bucket, key, off, length)
}

// _ checks that the cold store only receives ranged object reads.
var _ block_store_s3.ColdPackRangeReader = (*coldPackRangeReader)(nil)
