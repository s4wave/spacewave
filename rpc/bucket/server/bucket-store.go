package rpc_bucket_server

import (
	"context"

	rpc_bucket "github.com/aperturerobotics/bldr/rpc/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
)

// BucketStore implements the server with a BucketStore.
type BucketStore struct {
	// store is the underlying bucket store
	store bucket_store.Store
}

// NewBucketStore constructs a new BucketStore.
func NewBucketStore(store bucket_store.Store) *BucketStore {
	return &BucketStore{store: store}
}

// GetBucketConfig looks up the bucket config with the bucket id.
func (b *BucketStore) GetBucketConfig(ctx context.Context, req *rpc_bucket.GetBucketConfigRequest) (*rpc_bucket.GetBucketConfigResponse, error) {
	bucketConf, err := b.store.GetBucketConfig(req.GetBucketId())
	if err != nil {
		return nil, err
	}
	return &rpc_bucket.GetBucketConfigResponse{Config: bucketConf}, nil
}

// ApplyBucketConfig applies the bucket config to the store.
func (b *BucketStore) ApplyBucketConfig(
	ctx context.Context,
	req *rpc_bucket.ApplyBucketConfigRequest,
) (*rpc_bucket.ApplyBucketConfigResponse, error) {
	if err := req.GetConfig().Validate(); err != nil {
		return nil, err
	}
	updated, prev, curr, err := b.store.ApplyBucketConfig(req.GetConfig())
	if err != nil {
		return nil, err
	}
	return &rpc_bucket.ApplyBucketConfigResponse{
		Updated: updated,
		Prev:    prev,
		Curr:    curr,
	}, nil
}

// GetBucketInfo returns information about a bucket.
func (b *BucketStore) GetBucketInfo(ctx context.Context, req *rpc_bucket.GetBucketInfoRequest) (*rpc_bucket.GetBucketInfoResponse, error) {
	info, err := b.store.GetBucketInfo(req.GetBucketId())
	if err != nil {
		return nil, err
	}
	return &rpc_bucket.GetBucketInfoResponse{
		BucketInfo: info,
	}, nil
}

// ListBucketInfo lists buckets in the store.
func (b *BucketStore) ListBucketInfo(ctx context.Context, req *rpc_bucket.ListBucketInfoRequest) (*rpc_bucket.ListBucketInfoResponse, error) {
	re, err := req.ParseBucketIdRe()
	if err != nil {
		return nil, err
	}
	infos, err := b.store.ListBucketInfo(re)
	if err != nil {
		return nil, err
	}
	return &rpc_bucket.ListBucketInfoResponse{
		BucketInfo: infos,
	}, nil
}

// _ is a type assertion
var _ rpc_bucket.SRPCBucketStoreServer = ((*BucketStore)(nil))
