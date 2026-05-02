package sdk_world_engine

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
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
)

type sdkBucketLookupStore struct {
	service s4wave_bucket_lookup.SRPCBucketLookupCursorResourceServiceClient
	xfrm    block.Transformer
}

func newSDKBucketLookupCursor(
	ctx context.Context,
	ref resource_client.ResourceRef,
) (*bucket_lookup.Cursor, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	service := s4wave_bucket_lookup.NewSRPCBucketLookupCursorResourceServiceClient(srpcClient)
	resp, err := service.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{})
	if err != nil {
		return nil, err
	}
	objRef := resp.GetRef()
	store := &sdkBucketLookupStore{service: service}
	conf, xfrm, err := buildSDKCursorTransform(ctx, store, objRef)
	if err != nil {
		return nil, err
	}
	store.xfrm = xfrm
	var once sync.Once
	return bucket_lookup.NewCursorWithRelease(
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
	), nil
}

func accessSDKBucketLookupCursor(
	ctx context.Context,
	client *resource_client.Client,
	resourceID uint32,
	cb func(*bucket_lookup.Cursor) error,
) error {
	ref := client.CreateResourceReference(resourceID)
	cursor, err := newSDKBucketLookupCursor(ctx, ref)
	if err != nil {
		ref.Release()
		return err
	}
	defer cursor.Release()
	return cb(cursor)
}

func buildSDKCursorTransform(
	ctx context.Context,
	store *sdkBucketLookupStore,
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

func (s *sdkBucketLookupStore) GetHashType() hash.HashType {
	return 0
}

func (s *sdkBucketLookupStore) GetSupportedFeatures() block.StoreFeature {
	return 0
}

func (s *sdkBucketLookupStore) PutBlock(
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
	resp, err := s.service.PutBlock(ctx, &s4wave_bucket_lookup.PutBlockRequest{
		Data: data,
		Opts: opts,
	})
	if err != nil {
		return nil, false, err
	}
	return resp.GetRef(), resp.GetExisted(), nil
}

func (s *sdkBucketLookupStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			if err := s.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		_, _, err := s.PutBlock(ctx, entry.Data, &block.PutOpts{
			ForceBlockRef: entry.Ref.Clone(),
			Refs:          entry.Refs,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *sdkBucketLookupStore) PutBlockBackground(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	return s.PutBlock(ctx, data, opts)
}

func (s *sdkBucketLookupStore) GetBlock(
	ctx context.Context,
	ref *block.BlockRef,
) ([]byte, bool, error) {
	resp, err := s.service.GetBlock(ctx, &s4wave_bucket_lookup.GetBlockRequest{Ref: ref})
	if err != nil {
		return nil, false, err
	}
	data := resp.GetData()
	if resp.GetFound() && s.xfrm != nil {
		data, err = s.xfrm.EncodeBlock(data)
		if err != nil {
			return nil, true, err
		}
	}
	return data, resp.GetFound(), nil
}

func (s *sdkBucketLookupStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

func (s *sdkBucketLookupStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

func (s *sdkBucketLookupStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return errors.New("bucket lookup cursor resource does not support removing blocks")
}

func (s *sdkBucketLookupStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

func (s *sdkBucketLookupStore) Flush(ctx context.Context) error {
	return nil
}

func (s *sdkBucketLookupStore) BeginDeferFlush() {}

func (s *sdkBucketLookupStore) EndDeferFlush(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ bucket.BucketOps = (*sdkBucketLookupStore)(nil)
