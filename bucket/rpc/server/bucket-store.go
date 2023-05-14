package bucket_rpc_server

import (
	"context"

	bucket_rpc "github.com/aperturerobotics/hydra/bucket/rpc"
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
func (b *BucketStore) GetBucketConfig(ctx context.Context, req *bucket_rpc.GetBucketConfigRequest) (*bucket_rpc.GetBucketConfigResponse, error) {
	bucketConf, err := b.store.GetBucketConfig(ctx, req.GetBucketId())
	if err != nil {
		return nil, err
	}
	return &bucket_rpc.GetBucketConfigResponse{Config: bucketConf}, nil
}

// ApplyBucketConfig applies the bucket config to the store.
func (b *BucketStore) ApplyBucketConfig(
	ctx context.Context,
	req *bucket_rpc.ApplyBucketConfigRequest,
) (*bucket_rpc.ApplyBucketConfigResponse, error) {
	if err := req.GetConfig().Validate(); err != nil {
		return nil, err
	}
	updated, prev, curr, err := b.store.ApplyBucketConfig(ctx, req.GetConfig())
	if err != nil {
		return nil, err
	}
	return &bucket_rpc.ApplyBucketConfigResponse{
		Updated: updated,
		Prev:    prev,
		Curr:    curr,
	}, nil
}

// GetBucketInfo returns information about a bucket.
func (b *BucketStore) GetBucketInfo(ctx context.Context, req *bucket_rpc.GetBucketInfoRequest) (*bucket_rpc.GetBucketInfoResponse, error) {
	info, err := b.store.GetBucketInfo(ctx, req.GetBucketId())
	if err != nil {
		return nil, err
	}
	return &bucket_rpc.GetBucketInfoResponse{
		BucketInfo: info,
	}, nil
}

// ListBucketInfo lists buckets in the store.
func (b *BucketStore) ListBucketInfo(ctx context.Context, req *bucket_rpc.ListBucketInfoRequest) (*bucket_rpc.ListBucketInfoResponse, error) {
	re, err := req.ParseBucketIdRe()
	if err != nil {
		return nil, err
	}
	infos, err := b.store.ListBucketInfo(ctx, re)
	if err != nil {
		return nil, err
	}
	return &bucket_rpc.ListBucketInfoResponse{
		BucketInfo: infos,
	}, nil
}

// _ is a type assertion
var _ bucket_rpc.SRPCBucketStoreServer = ((*BucketStore)(nil))
