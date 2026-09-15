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

// GetHashType selects the storage default.
func (s *cursorStore) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures advertises the cursor's batch RPCs.
func (s *cursorStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists
}

// BeginReadOperation reuses this cursor without acquiring another resource.
func (s *cursorStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock sends decoded content for the server to transform and store.
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

// PutBlockBatch preserves entry order across bounded decoded requests.
func (s *cursorStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Bound decoded payloads: compressed blocks can expand far beyond the
	// caller's write buffer. Leave room below the Resource packet limit.
	const batchBytes = 4 << 20
	req := &PutBlockBatchRequest{}
	size := 0
	flush := func() error {
		if len(req.Entries) == 0 {
			return nil
		}
		_, err := s.service.PutBlockBatch(ctx, req)
		if err != nil {
			return errors.Wrapf(err, "write %d blocks (%d bytes)", len(req.Entries), size)
		}
		req = &PutBlockBatchRequest{}
		size = 0
		return err
	}
	for _, entry := range entries {
		data := entry.Data
		if !entry.Tombstone && s.xfrm != nil {
			var err error
			data = bytes.Clone(data)
			data, err = s.xfrm.DecodeBlock(data)
			if err != nil {
				return err
			}
		}
		decoded := &PutBlockBatchEntry{
			Ref:       entry.Ref,
			Data:      data,
			Refs:      entry.Refs,
			Tombstone: entry.Tombstone,
		}
		// Ten bytes cover the repeated field's tag and length prefix.
		entrySize := decoded.SizeVT() + 10
		if size+entrySize > batchBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		req.Entries = append(req.Entries, decoded)
		size += entrySize
	}
	return flush()
}

// GetBlockExistsBatch returns one existence result per requested reference.
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

// GetBlock restores the storage transform after reading decoded remote content.
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

// GetBlockExists reports whether the cursor can resolve the block.
func (s *cursorStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

// RmBlock rejects removal, which the cursor Resource does not expose.
func (s *cursorStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return errors.New("bucket lookup cursor resource does not support removing blocks")
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

// Sync completes immediately because each cursor RPC finishes its write.
func (s *cursorStore) Sync(ctx context.Context) (bool, error) {
	return true, nil
}

var _ bucket.BucketOps = (*cursorStore)(nil)
