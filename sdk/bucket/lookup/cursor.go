package s4wave_bucket_lookup

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// ResourceClient creates references for resource IDs returned by RPCs.
type ResourceClient interface {
	// CreateResourceReference acquires a reference the caller must release.
	CreateResourceReference(resourceID uint32) resource_client.ResourceRef
}

// NewCursor wraps a bucket lookup cursor resource reference in a local
// bucket_lookup.Cursor backed by RPC calls to the cursor resource.
// The returned cursor releases the reference when released.
func NewCursor(
	ctx context.Context,
	ref resource_client.ResourceRef,
) (*bucket_lookup.Cursor, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	service := NewSRPCBucketLookupCursorResourceServiceClient(srpcClient)
	resp, err := service.GetRef(ctx, &GetRefRequest{})
	if err != nil {
		return nil, err
	}
	objRef := resp.GetRef()
	store := &cursorStore{StoreOps: block_rpc_client.NewBlockStore(block_rpc.NewSRPCBlockStoreClient(srpcClient), 0, false)}
	conf, xfrm, err := buildCursorTransform(ctx, store, objRef)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	cursor := bucket_lookup.NewCursorWithRelease(
		ctx,
		nil,
		nil,
		nil,
		store,
		xfrm,
		objRef,
		&bucket.BucketOpArgs{BucketId: objRef.GetBucketId()},
		conf,
		func() {
			once.Do(ref.Release)
		},
	)
	cursor.SetBucketIDOverride(resp.GetBucketIdOverride())
	return cursor, nil
}

// AccessCursor resolves a cursor resource ID, invokes cb with the wrapped
// cursor, and releases the reference after cb returns.
func AccessCursor(
	ctx context.Context,
	client ResourceClient,
	resourceID uint32,
	cb func(*bucket_lookup.Cursor) error,
) error {
	ref := client.CreateResourceReference(resourceID)
	cursor, err := NewCursor(ctx, ref)
	if err != nil {
		ref.Release()
		return err
	}
	defer cursor.Release()
	return cb(cursor)
}

// buildCursorTransform resolves the transform configuration and transformer
// for the referenced object.
func buildCursorTransform(
	ctx context.Context,
	store block.StoreOps,
	objRef *bucket.ObjectRef,
) (*block_transform.Config, block.Transformer, error) {
	conf := objRef.GetTransformConf()
	if conf.GetEmpty() && !objRef.GetTransformConfRef().GetEmpty() {
		var err error
		conf, err = bucket_lookup.FetchTransformConf(ctx, store, objRef.GetTransformConfRef(), nil)
		if err != nil {
			return nil, nil, err
		}
	}
	if conf.GetEmpty() {
		return nil, nil, nil
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{},
		transform_all.BuildFactorySet(),
		conf,
	)
	if err != nil {
		return nil, nil, err
	}
	return conf, xfrm, nil
}

// cursorStore records Resource reads while the shared RPC client carries encoded blocks.
type cursorStore struct {
	block.StoreOps
}

// BeginReadOperation keeps Resource read accounting within the borrowed scope.
func (s *cursorStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// GetBlock returns stored bytes without applying the cursor transform.
func (s *cursorStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	if err == nil {
		block.RecordResourceGetBlock(ctx, ref, found, len(data))
	}
	return data, found, err
}

// StatBlock reports the transformed size, or nil when the block is absent.
func (s *cursorStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

// _ verifies the mounted bucket contract.
var _ bucket.BucketOps = (*cursorStore)(nil)
