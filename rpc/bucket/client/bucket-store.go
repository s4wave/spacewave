package rpc_bucket_client

import (
	"context"
	"regexp"

	rpc_bucket "github.com/aperturerobotics/bldr/rpc/bucket"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
)

// BucketStore implements a BucketStore backed by a BucketStore service.
type BucketStore struct {
	// ctx is used for volume lookups
	ctx context.Context
	// client is the client to use
	client rpc_bucket.SRPCBucketStoreClient
}

// NewBucketStore constructs a new BucketStore.
func NewBucketStore(
	ctx context.Context,
	client rpc_bucket.SRPCBucketStoreClient,
) *BucketStore {
	return &BucketStore{
		ctx:    ctx,
		client: client,
	}
}

// GetBucketConfig gets the bucket config for the bucket ID.
// Can return nil if no bucket config is found.
func (v *BucketStore) GetBucketConfig(id string) (*bucket.Config, error) {
	resp, err := v.client.GetBucketConfig(v.ctx, &rpc_bucket.GetBucketConfigRequest{
		BucketId: id,
	})
	if err != nil {
		return nil, err
	}
	if resp.GetConfig().GetId() == "" {
		return nil, nil
	}
	return resp.GetConfig(), nil
}

// ApplyBucketConfig applies a bucket configuration.
// Returns the previous and current (updated) configurations.
// The current configuration may be nil if the volume rejects the bucket.
// If outdated, prev == curr.
func (v *BucketStore) ApplyBucketConfig(
	conf *bucket.Config,
) (updated bool, prev, curr *bucket.Config, err error) {
	resp, err := v.client.ApplyBucketConfig(v.ctx, &rpc_bucket.ApplyBucketConfigRequest{
		Config: conf.CloneVT(),
	})
	if err != nil {
		return false, nil, nil, err
	}
	return resp.GetUpdated(), resp.GetPrev(), resp.GetCurr(), nil
}

// GetBucketInfo returns bucket information by ID.
func (v *BucketStore) GetBucketInfo(id string) (*bucket.BucketInfo, error) {
	resp, err := v.client.GetBucketInfo(v.ctx, &rpc_bucket.GetBucketInfoRequest{
		BucketId: id,
	})
	if err != nil {
		return nil, err
	}
	if resp.GetBucketInfo().GetConfig().GetId() == "" {
		return nil, nil
	}
	return resp.GetBucketInfo(), nil
}

// ListBucketInfo lists buckets with an optional regex match.
func (v *BucketStore) ListBucketInfo(idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	var idReStr string
	if idRegex != nil {
		idReStr = idRegex.String()
	}
	resp, err := v.client.ListBucketInfo(v.ctx, &rpc_bucket.ListBucketInfoRequest{
		BucketIdRe: idReStr,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetBucketInfo(), nil
}

// GetReconcilerEventQueue returns a reference to the event queue for a
// reconciler ID. Should not return nil without an error.
func (v *BucketStore) GetReconcilerEventQueue(bucket_store.BucketReconcilerPair) (mqueue.Queue, error) {
	return nil, rpc_bucket.ErrReconcilerUnavailable
}

// DeleteReconcilerEventQueue purges a reconciler event queue.
func (v *BucketStore) DeleteReconcilerEventQueue(bucket_store.BucketReconcilerPair) error {
	return rpc_bucket.ErrReconcilerUnavailable
}

// ListFilledReconcilerEventQueues lists reconciler event queues that have
// at least one event, by reconciler ID.
func (v *BucketStore) ListFilledReconcilerEventQueues() ([]bucket_store.BucketReconcilerPair, error) {
	return nil, rpc_bucket.ErrReconcilerUnavailable
}

// _ is a type assertion
var _ bucket_store.Store = ((*BucketStore)(nil))
