package bucket_store_rpc_client

import (
	"context"
	"regexp"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	bucket_store_rpc "github.com/s4wave/spacewave/db/bucket/store/rpc"
	"github.com/s4wave/spacewave/db/mqueue"
)

// BucketStore implements a BucketStore backed by a BucketStore service.
type BucketStore struct {
	// client is the client to use
	client bucket_store_rpc.SRPCBucketStoreClient
}

// NewBucketStore constructs a new BucketStore.
func NewBucketStore(client bucket_store_rpc.SRPCBucketStoreClient) *BucketStore {
	return &BucketStore{client: client}
}

// GetBucketConfig gets the bucket config for the bucket ID.
// Can return nil if no bucket config is found.
func (v *BucketStore) GetBucketConfig(ctx context.Context, id string) (*bucket.Config, error) {
	resp, err := v.client.GetBucketConfig(ctx, &bucket_store_rpc.GetBucketConfigRequest{
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
	ctx context.Context,
	conf *bucket.Config,
) (updated bool, prev, curr *bucket.Config, err error) {
	resp, err := v.client.ApplyBucketConfig(ctx, &bucket_store_rpc.ApplyBucketConfigRequest{
		Config: conf.CloneVT(),
	})
	if err != nil {
		return false, nil, nil, err
	}
	return resp.GetUpdated(), resp.GetPrev(), resp.GetCurr(), nil
}

// GetBucketInfo returns bucket information by ID.
func (v *BucketStore) GetBucketInfo(ctx context.Context, id string) (*bucket.BucketInfo, error) {
	resp, err := v.client.GetBucketInfo(ctx, &bucket_store_rpc.GetBucketInfoRequest{
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
func (v *BucketStore) ListBucketInfo(ctx context.Context, idRegex *regexp.Regexp) ([]*bucket.BucketInfo, error) {
	var idReStr string
	if idRegex != nil {
		idReStr = idRegex.String()
	}
	resp, err := v.client.ListBucketInfo(ctx, &bucket_store_rpc.ListBucketInfoRequest{
		BucketIdRe: idReStr,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetBucketInfo(), nil
}

// GetReconcilerEventQueue returns a reference to the event queue for a
// reconciler ID. Should not return nil without an error.
func (v *BucketStore) GetReconcilerEventQueue(ctx context.Context, p bucket_store.BucketReconcilerPair) (mqueue.Queue, error) {
	return nil, bucket_store_rpc.ErrReconcilerUnavailable
}

// DeleteReconcilerEventQueue purges a reconciler event queue.
func (v *BucketStore) DeleteReconcilerEventQueue(ctx context.Context, p bucket_store.BucketReconcilerPair) error {
	return bucket_store_rpc.ErrReconcilerUnavailable
}

// ListFilledReconcilerEventQueues lists reconciler event queues that have
// at least one event, by reconciler ID.
func (v *BucketStore) ListFilledReconcilerEventQueues(ctx context.Context) ([]bucket_store.BucketReconcilerPair, error) {
	return nil, bucket_store_rpc.ErrReconcilerUnavailable
}

// _ is a type assertion
var _ bucket_store.Store = ((*BucketStore)(nil))
