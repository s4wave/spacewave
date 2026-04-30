//go:build !sql_lite

package bucket

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
)

// AccessBucketFunc is a function to access a BucketHandle.
// Optionally pass a released function that may be called when the handle was released.
// Returns a release function.
type AccessBucketFunc = func(ctx context.Context, released func()) (BucketHandle, func(), error)

// NewAccessBucketViaBusFunc builds a new func which accesses the Bucket on the
// given bus using the AccessBucket directive.
//
// If returnIfIdle is set: ErrBucketNotFound is returned if not found.
func NewAccessBucketViaBusFunc(b bus.Bus, bucketID, bucketStoreID string, returnIfIdle bool) AccessBucketFunc {
	return func(ctx context.Context, released func()) (BucketHandle, func(), error) {
		// access the directive via the bus
		val, _, ref, err := ExBuildBucketAPI(ctx, b, returnIfIdle, bucketID, bucketStoreID, released)
		if err != nil || val == nil {
			if ref != nil {
				ref.Release()
			}
			if err == nil {
				err = ErrBucketNotFound
			}
			return nil, nil, err
		}

		return val, ref.Release, nil
	}
}
