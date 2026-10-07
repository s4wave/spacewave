package volume_scoped

import (
	"context"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
)

// bucketStore serves the buckets of a scoped volume view.
//
// Every bucket ID gains the view prefix on the way in and loses it on the way
// out, and listings show only the buckets of the view.
type bucketStore struct {
	// inner is the bucket store of the underlying volume.
	inner bucket_store.Store
	// prefix is the view prefix.
	prefix string
}

// ApplyBucketConfig applies a bucket configuration inside the view. It refuses
// a lookup controller, which would start a controller on the host.
func (b *bucketStore) ApplyBucketConfig(ctx context.Context, conf *bucket.Config) (bool, *bucket.Config, *bucket.Config, error) {
	// Refuse a lookup controller before touching the volume.
	if conf.GetLookup().GetController() != nil {
		return false, nil, nil, errors.Wrap(ErrRefused, "bucket lookup controller")
	}

	// Apply the config under the prefixed bucket ID.
	scoped := conf.CloneVT()
	if scoped != nil {
		scoped.Id = b.prefix + scoped.GetId()
	}

	// Report the previous and current configs without the prefix.
	updated, prev, curr, err := b.inner.ApplyBucketConfig(ctx, scoped)
	if err != nil {
		return false, nil, nil, err
	}
	return updated, b.unscopeConfig(prev), b.unscopeConfig(curr), nil
}

// GetBucketConfig gets the bucket config for the bucket ID in the view.
func (b *bucketStore) GetBucketConfig(ctx context.Context, id string) (*bucket.Config, error) {
	conf, err := b.inner.GetBucketConfig(ctx, b.prefix+id)
	if err != nil {
		return nil, err
	}
	return b.unscopeConfig(conf), nil
}

// GetBucketInfo returns bucket information by bucket ID in the view.
func (b *bucketStore) GetBucketInfo(ctx context.Context, id string) (*bucket.BucketInfo, error) {
	info, err := b.inner.GetBucketInfo(ctx, b.prefix+id)
	if err != nil {
		return nil, err
	}
	return b.unscopeInfo(info), nil
}

// ListBucketInfo lists the buckets of the view, matching the regex against the
// bucket ID without the prefix.
func (b *bucketStore) ListBucketInfo(ctx context.Context, idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	// Match the volume buckets under the prefix.
	prefixRegex, err := regexp.Compile("^" + regexp.QuoteMeta(b.prefix))
	if err != nil {
		return nil, err
	}

	// List the volume buckets under the prefix.
	infos, err := b.inner.ListBucketInfo(ctx, prefixRegex)
	if err != nil {
		return nil, err
	}

	// Apply the caller's filter to the bucket ID without the prefix.
	listed := make([]*bucket.BucketInfo, 0, len(infos))
	for _, info := range infos {
		unscoped := b.unscopeInfo(info)
		if idRegex == nil || idRegex.MatchString(unscoped.GetConfig().GetId()) {
			listed = append(listed, unscoped)
		}
	}
	return listed, nil
}

// unscopeConfig returns a copy of conf with the view prefix removed from its
// bucket ID. It returns nil for a nil conf.
func (b *bucketStore) unscopeConfig(conf *bucket.Config) *bucket.Config {
	unscoped := conf.CloneVT()
	if unscoped != nil {
		unscoped.Id = strings.TrimPrefix(unscoped.GetId(), b.prefix)
	}
	return unscoped
}

// unscopeInfo returns a copy of info with the view prefix removed from its
// bucket ID. It returns nil for a nil info.
func (b *bucketStore) unscopeInfo(info *bucket.BucketInfo) *bucket.BucketInfo {
	if info == nil {
		return nil
	}
	return &bucket.BucketInfo{Config: b.unscopeConfig(info.GetConfig())}
}

// _ is a type assertion
var _ bucket_store.Store = (*bucketStore)(nil)
