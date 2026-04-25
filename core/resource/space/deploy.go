package resource_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	s4wave_deploy "github.com/s4wave/spacewave/sdk/deploy"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"

	"github.com/s4wave/spacewave/net/hash"
)

// DeployManifest handles the bidirectional deploy manifest stream.
func (r *SpaceResource) DeployManifest(strm s4wave_space.SRPCSpaceResourceService_DeployManifestStream) error {
	ctx := strm.Context()

	// Receive initial request.
	msg, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv initial request")
	}
	req := msg.GetRequest()
	if req == nil {
		return errors.New("first message must be DeployManifestRequest")
	}

	manifestRef := req.GetManifestRef()
	objectKey := req.GetObjectKey()
	manifestID := req.GetManifestId()
	rootRef := manifestRef.GetRootRef()

	r.le.Infof("deploy manifest: manifest=%s key=%s ref=%s",
		manifestID, objectKey, rootRef.MarshalString())

	if rootRef.GetEmpty() {
		return sendDeployResult(strm, "manifest_ref root_ref is required")
	}
	if manifestID == "" {
		return sendDeployResult(strm, "manifest_id is required")
	}

	// Build a block transformer from the ObjectRef's transform config.
	// This allows decoding s2-compressed blocks for DAG traversal.
	var xfrm block.Transformer
	if tc := manifestRef.GetTransformConf(); tc != nil && len(tc.GetSteps()) > 0 {
		sfs := block_transform.NewStepFactorySet()
		sfs.AddStepFactory(transform_s2.NewStepFactory())
		xfrm, err = block_transform.NewTransformer(
			controller.ConstructOpts{Logger: r.le},
			sfs,
			tc,
		)
		if err != nil {
			r.le.WithError(err).Warn("build block transformer failed")
			return sendDeployResult(strm, "invalid transform config: "+err.Error())
		}
	}

	engine := r.space.GetWorldEngine()

	// Build a storage cursor to access the block store for writing.
	cursor, err := engine.BuildStorageCursor(ctx)
	if err != nil {
		r.le.WithError(err).Warn("build storage cursor failed")
		return sendDeployResult(strm, err.Error())
	}
	defer cursor.Release()

	// Use the raw bucket as the destination (no transform applied).
	// Blocks are stored as-is (compressed) to preserve block refs.
	dest := cursor.GetBucket()

	// Build a StoreOps adapter that reads blocks from the client stream.
	src := &streamStoreOps{strm: strm}

	// Copy the manifest block DAG from stream source to dest.
	// Uses the transformer to decode blocks for protobuf traversal only.
	err = copyBlockDAGWithTransform(ctx, rootRef, bldr_manifest.NewManifestBlock, src, dest, xfrm)
	if err != nil {
		r.le.WithError(err).Warn("deploy manifest: block copy failed")
		return sendDeployResult(strm, err.Error())
	}

	// Create the manifest object in the Space world.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		r.le.WithError(err).Warn("deploy manifest: new transaction failed")
		return sendDeployResult(strm, err.Error())
	}
	defer tx.Discard()

	// Store the ObjectRef with transform config so the world can decode blocks later.
	objRef := &bucket.ObjectRef{
		BucketId:      r.space.GetWorldEngineBucketID(),
		RootRef:       rootRef,
		TransformConf: manifestRef.GetTransformConf(),
	}
	_, _, err = bldr_manifest_world.SetManifest(ctx, tx, "", objectKey, objRef)
	if err != nil {
		r.le.WithError(err).Warn("deploy manifest: set manifest failed")
		return sendDeployResult(strm, err.Error())
	}

	err = tx.Commit(ctx)
	if err != nil {
		r.le.WithError(err).Warn("deploy manifest: commit failed")
		return sendDeployResult(strm, err.Error())
	}

	r.le.Infof("deploy manifest complete: manifest=%s key=%s", manifestID, objectKey)
	return sendDeployResult(strm, "")
}

// sendDeployResult sends a DeployManifestResult on the stream and closes it.
func sendDeployResult(strm s4wave_space.SRPCSpaceResourceService_DeployManifestStream, errMsg string) error {
	return strm.SendAndClose(&s4wave_deploy.DeployManifestMessage{
		Body: &s4wave_deploy.DeployManifestMessage_Result{
			Result: &s4wave_deploy.DeployManifestResult{
				Error: errMsg,
			},
		},
	})
}

// copyBlockDAGWithTransform copies all blocks reachable from rootRef from src
// to dest, decoding blocks with xfrm before protobuf unmarshal for DAG traversal.
// Raw (possibly compressed) data is written to dest as-is to preserve block refs.
func copyBlockDAGWithTransform(
	ctx context.Context,
	rootRef *block.BlockRef,
	rootCtor block.Ctor,
	src block.StoreOps,
	dest block.StoreOps,
	xfrm block.Transformer,
) error {
	if rootRef.GetEmpty() {
		return nil
	}
	visited := make(map[string]bool)
	return copyBlockWithTransform(ctx, rootRef, rootCtor, src, dest, xfrm, visited)
}

// copyBlockWithTransform copies a single block and recursively copies its children.
func copyBlockWithTransform(
	ctx context.Context,
	ref *block.BlockRef,
	ctor block.Ctor,
	src, dest block.StoreOps,
	xfrm block.Transformer,
	visited map[string]bool,
) error {
	if ref.GetEmpty() {
		return nil
	}

	refStr := ref.MarshalString()
	if visited[refStr] {
		return nil
	}
	visited[refStr] = true

	// Check if already in dest.
	exists, err := dest.GetBlockExists(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "check block exists: %s", refStr)
	}
	if exists {
		return nil
	}

	// Read raw (possibly compressed) data from source.
	data, found, err := src.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "get block: %s", refStr)
	}
	if !found {
		return errors.Wrapf(block.ErrNotFound, "block: %s", refStr)
	}

	// Write raw data to dest (preserves block refs).
	if _, _, err := dest.PutBlock(ctx, data, nil); err != nil {
		return errors.Wrapf(err, "put block: %s", refStr)
	}

	// No constructor means we can't traverse children (leaf copy).
	if ctor == nil {
		return nil
	}

	// Decode for protobuf unmarshal (decompress if needed).
	decoded := data
	if xfrm != nil {
		decoded, err = xfrm.DecodeBlock(data)
		if err != nil {
			return errors.Wrapf(err, "decode block: %s", refStr)
		}
	}

	blk := ctor()
	if err := blk.UnmarshalBlock(decoded); err != nil {
		return errors.Wrapf(err, "unmarshal block: %s", refStr)
	}

	// Follow child block refs.
	if err := followRefsWithTransform(ctx, blk, src, dest, xfrm, visited); err != nil {
		return err
	}

	// Check sub-blocks for refs too.
	if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
		for _, sub := range withSubBlocks.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			if err := followRefsWithTransform(ctx, sub, src, dest, xfrm, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

// followRefsWithTransform checks if blk implements BlockWithRefs and recursively copies children.
func followRefsWithTransform(
	ctx context.Context,
	blk any,
	src, dest block.StoreOps,
	xfrm block.Transformer,
	visited map[string]bool,
) error {
	withRefs, ok := blk.(block.BlockWithRefs)
	if !ok {
		return nil
	}
	refs, err := withRefs.GetBlockRefs()
	if err != nil {
		return errors.Wrap(err, "get block refs")
	}
	for id, childRef := range refs {
		childCtor := withRefs.GetBlockRefCtor(id)
		if err := copyBlockWithTransform(ctx, childRef, childCtor, src, dest, xfrm, visited); err != nil {
			return err
		}
	}
	return nil
}

// streamStoreOps implements block.StoreOps by requesting blocks over the stream.
type streamStoreOps struct {
	strm s4wave_space.SRPCSpaceResourceService_DeployManifestStream
}

// GetHashType returns the preferred hash type for the store.
func (s *streamStoreOps) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitset.
func (s *streamStoreOps) GetSupportedFeatures() block.StoreFeature {
	return 0
}

// GetBlock requests a block from the client over the stream.
func (s *streamStoreOps) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	err := s.strm.Send(&s4wave_deploy.DeployManifestMessage{
		Body: &s4wave_deploy.DeployManifestMessage_BlockRequest{
			BlockRequest: &s4wave_deploy.BlockRequest{
				Ref: ref,
			},
		},
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "send block request")
	}

	msg, err := s.strm.Recv()
	if err != nil {
		return nil, false, errors.Wrap(err, "recv block response")
	}
	resp := msg.GetBlockResponse()
	if resp == nil {
		return nil, false, errors.New("expected BlockResponse")
	}
	if resp.GetNotFound() {
		return nil, false, nil
	}
	return resp.GetData(), true, nil
}

// GetBlockExists checks if a block exists.
// Always returns false to let the copy function fetch from source.
func (s *streamStoreOps) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch returns false for every ref to force source reads.
func (s *streamStoreOps) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// PutBlock is not used on the source side.
func (s *streamStoreOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch is not used on the source side.
func (s *streamStoreOps) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return block_store.ErrReadOnly
}

// PutBlockBackground is not used on the source side.
func (s *streamStoreOps) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil (unsupported on stream source).
func (s *streamStoreOps) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

// RmBlock is not used on the source side.
func (s *streamStoreOps) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Flush has no buffered work for the stream source.
func (s *streamStoreOps) Flush(_ context.Context) error {
	return nil
}

// BeginDeferFlush is a no-op for the stream source.
func (s *streamStoreOps) BeginDeferFlush() {}

// EndDeferFlush is a no-op for the stream source.
func (s *streamStoreOps) EndDeferFlush(_ context.Context) error {
	return nil
}

// _ is a type assertion
var _ block.StoreOps = ((*streamStoreOps)(nil))
