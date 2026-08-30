package s4wave_bucket_lookup

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/hash"
)

// ResourceClient creates references for resource IDs returned by RPCs.
type ResourceClient interface {
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
	store := &cursorStore{service: service}
	conf, xfrm, err := buildCursorTransform(ctx, store, objRef)
	if err != nil {
		return nil, err
	}
	store.xfrm = xfrm
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
	store *cursorStore,
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

// cursorStore serves block reads over the bucket lookup cursor resource RPCs.
type cursorStore struct {
	service SRPCBucketLookupCursorResourceServiceClient
	xfrm    block.Transformer
}

func (s *cursorStore) GetHashType() hash.HashType {
	return 0
}

func (s *cursorStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists
}

func (s *cursorStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *cursorStore) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	var err error
	if s.xfrm != nil {
		data = bytes.Clone(data)
		data, err = s.xfrm.DecodeBlock(data)
		if err != nil {
			return nil, false, err
		}
	}
	resp, err := s.service.PutBlock(ctx, &PutBlockRequest{
		Data: data,
		Opts: opts,
	})
	if err != nil {
		return nil, false, err
	}
	return resp.GetRef(), resp.GetExisted(), nil
}

func (s *cursorStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	reqEntries := make([]*PutBlockBatchEntry, len(entries))
	for i, entry := range entries {
		data := entry.Data
		if !entry.Tombstone && s.xfrm != nil {
			var err error
			data = bytes.Clone(data)
			data, err = s.xfrm.DecodeBlock(data)
			if err != nil {
				return err
			}
		}
		reqEntries[i] = &PutBlockBatchEntry{
			Ref:       entry.Ref,
			Data:      data,
			Refs:      entry.Refs,
			Tombstone: entry.Tombstone,
		}
	}
	_, err := s.service.PutBlockBatch(ctx, &PutBlockBatchRequest{
		Entries: reqEntries,
	})
	return err
}

func (s *cursorStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	resp, err := s.service.GetBlockExistsBatch(ctx, &GetBlockExistsBatchRequest{
		Refs: refs,
	})
	if err != nil {
		return nil, err
	}
	found := resp.GetFound()
	if len(found) != len(refs) {
		return nil, errors.Errorf("bucket lookup cursor resource returned %d existence results for %d refs", len(found), len(refs))
	}
	return found, nil
}

func (s *cursorStore) GetBlock(
	ctx context.Context,
	ref *block.BlockRef,
) ([]byte, bool, error) {
	resp, err := s.service.GetBlock(ctx, &GetBlockRequest{Ref: ref})
	if err != nil {
		return nil, false, err
	}
	data := resp.GetData()
	block.RecordResourceGetBlock(ctx, ref, resp.GetFound(), len(data))
	if resp.GetFound() && s.xfrm != nil {
		data, err = s.xfrm.EncodeBlock(data)
		if err != nil {
			return nil, true, err
		}
	}
	return data, resp.GetFound(), nil
}

func (s *cursorStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

func (s *cursorStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return errors.New("bucket lookup cursor resource does not support removing blocks")
}

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

func (s *cursorStore) Sync(ctx context.Context) (bool, error) {
	return true, nil
}

var _ bucket.BucketOps = (*cursorStore)(nil)
