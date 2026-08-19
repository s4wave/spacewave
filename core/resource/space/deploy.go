package resource_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_deploy "github.com/s4wave/spacewave/sdk/deploy"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// DeployManifests handles the bidirectional manifest-set deployment stream.
func (r *SpaceResource) DeployManifests(strm s4wave_space.SRPCSpaceResourceService_DeployManifestsStream) error {
	ctx := strm.Context()

	// Receive the initial request message before inspecting its deployment shape.
	msg, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv initial request")
	}
	req := msg.GetRequest()
	if req == nil {
		return errors.New("first message must be DeployManifestsRequest")
	}

	// Validate the request shape and complete manifest set before touching the World.
	objectKey := req.GetObjectKey()
	refs := req.GetManifestRefs()
	if objectKey == "" {
		return sendDeployManifestsResult(strm, "object_key is required")
	}
	if len(refs) == 0 {
		return sendDeployManifestsResult(strm, "manifest_refs is required")
	}

	// Validate the complete request before reading or writing any block.
	manifestID, err := validateManifestSet(refs)
	if err != nil {
		return sendDeployManifestsResult(strm, err.Error())
	}

	// Reject a known wrong host before block transfer; repeat this check in the write transaction.
	engine := r.space.GetWorldEngine()
	ws := world.NewEngineWorldState(engine, false)
	if _, exists, err := ws.GetObject(ctx, objectKey); err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "check manifest store").Error())
	} else if exists {
		if err := bldr_manifest_world.CheckManifestStoreType(ctx, ws, objectKey); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrap(err, "manifest store type").Error())
		}
	}

	// Copy and validate every manifest DAG before opening the transaction.
	cursor, err := engine.BuildStorageCursor(ctx)
	if err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "build storage cursor").Error())
	}
	defer cursor.Release()
	dest := cursor.GetBucket()
	src := &streamStoreOps{strm: strm}
	visited := make(map[string]bool)
	storedRefs := make([]*bucket.ObjectRef, len(refs))
	for i, ref := range refs {
		if err := ctx.Err(); err != nil {
			return sendDeployManifestsResult(strm, err.Error())
		}
		xfrm, err := newManifestTransformer(r.le, ref.GetManifestRef().GetTransformConf())
		if err != nil {
			return sendDeployManifestsResult(strm, errors.Wrapf(err, "manifest_refs[%d] transform", i).Error())
		}
		rootRef := ref.GetManifestRef().GetRootRef()
		if err := copyBlockWithTransform(ctx, rootRef, bldr_manifest.NewManifestBlock, src, dest, xfrm, visited); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrapf(err, "manifest_refs[%d] block copy", i).Error())
		}
		if err := validateCopiedManifest(ctx, dest, rootRef, ref.GetMeta(), xfrm); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrapf(err, "manifest_refs[%d] copied manifest", i).Error())
		}
		storedRefs[i] = &bucket.ObjectRef{
			BucketId:      r.space.GetWorldEngineBucketID(),
			RootRef:       rootRef,
			TransformConf: ref.GetManifestRef().GetTransformConf(),
		}
	}

	// Publish the host, child objects, and graph edges in one transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "new transaction").Error())
	}
	defer tx.Discard()
	txws := world.WorldState(tx)

	// Authoritatively verify or create the host store inside this transaction.
	if _, exists, err := txws.GetObject(ctx, objectKey); err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "check manifest store in transaction").Error())
	} else if exists {
		if err := bldr_manifest_world.CheckManifestStoreType(ctx, txws, objectKey); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrap(err, "manifest store type in transaction").Error())
		}
	} else if _, err := bldr_manifest_world.CreateManifestStore(ctx, txws, objectKey); err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "create manifest store").Error())
	}

	// Mutate deterministic child Manifest objects and their host edges.
	for i, ref := range refs {
		childKey := bldr_manifest.NewManifestKey(objectKey, ref.GetMeta())
		// Store the exact copied reference at the deterministic child key.
		if _, _, err := bldr_manifest_world.SetManifest(ctx, txws, "", childKey, storedRefs[i]); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrapf(err, "set manifest_refs[%d]", i).Error())
		}

		// Link the host to this child under the shared manifest ID.
		if err := txws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objectKey, childKey, manifestID)); err != nil {
			return sendDeployManifestsResult(strm, errors.Wrapf(err, "link manifest_refs[%d]", i).Error())
		}
	}

	// Commit is the publication point; Sync and the result may be ambiguous after it.
	if err := tx.Commit(ctx); err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "commit manifest set").Error())
	}
	if _, err := engine.Sync(ctx); err != nil {
		return sendDeployManifestsResult(strm, errors.Wrap(err, "sync manifest set").Error())
	}
	r.le.WithField("manifest-id", manifestID).WithField("object-key", objectKey).
		Info("deploy manifest set complete")
	return sendDeployManifestsResult(strm, "")
}

func validateManifestSet(refs []*bldr_manifest.ManifestRef) (string, error) {
	var manifestID string
	platforms := make(map[string]struct{}, len(refs))
	for i, ref := range refs {
		if ref == nil {
			return "", errors.Errorf("manifest_refs[%d] is required", i)
		}
		if err := ref.Validate(); err != nil {
			return "", errors.Wrapf(err, "manifest_refs[%d]", i)
		}
		if ref.GetManifestRef().GetRootRef().GetEmpty() {
			return "", errors.Errorf("manifest_refs[%d] root_ref is required", i)
		}
		id := ref.GetMeta().GetManifestId()
		if manifestID == "" {
			manifestID = id
		} else if id != manifestID {
			return "", errors.Errorf("manifest_refs[%d] has manifest ID %q, want %q", i, id, manifestID)
		}
		if _, err := bldr_platform.ParsePlatform(ref.GetMeta().GetPlatformId()); err != nil {
			return "", errors.Wrapf(err, "manifest_refs[%d] platform", i)
		}
		platformID := ref.GetMeta().GetPlatformId()
		if _, ok := platforms[platformID]; ok {
			return "", errors.Errorf("manifest_refs[%d] duplicates platform %q", i, platformID)
		}
		platforms[platformID] = struct{}{}
	}
	return manifestID, nil
}

func newManifestTransformer(le *logrus.Entry, tc *block_transform.Config) (block.Transformer, error) {
	if tc == nil || len(tc.GetSteps()) == 0 {
		return nil, nil
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	return block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, tc)
}

func validateCopiedManifest(
	ctx context.Context,
	dest block.StoreOps,
	rootRef *block.BlockRef,
	meta *bldr_manifest.ManifestMeta,
	xfrm block.Transformer,
) error {
	data, found, err := dest.GetBlock(ctx, rootRef)
	if err != nil {
		return err
	}
	if !found {
		return block.ErrNotFound
	}
	if xfrm != nil {
		data, err = xfrm.DecodeBlock(data)
		if err != nil {
			return err
		}
	}
	manifest := bldr_manifest.NewManifest(nil, "")
	if err := manifest.UnmarshalBlock(data); err != nil {
		return err
	}
	if !manifest.GetMeta().EqualVT(meta) {
		return errors.New("metadata differs from copied Manifest")
	}
	return manifest.Validate()
}

// sendDeployManifestsResult sends a result and closes the stream.
func sendDeployManifestsResult(strm s4wave_space.SRPCSpaceResourceService_DeployManifestsStream, errMsg string) error {
	return strm.SendAndClose(&s4wave_deploy.DeployManifestsMessage{
		Body: &s4wave_deploy.DeployManifestsMessage_Result{
			Result: &s4wave_deploy.DeployManifestsResult{Error: errMsg},
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

	var data []byte
	if exists {
		var found bool
		data, found, err = dest.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "get existing block: %s", refStr)
		}
		if !found {
			return errors.Wrapf(block.ErrNotFound, "existing block: %s", refStr)
		}
		actual, err := block.BuildBlockRef(data, &block.PutOpts{HashType: ref.GetHash().GetHashType()})
		if err != nil {
			return errors.Wrapf(err, "hash existing block: %s", refStr)
		}
		if !actual.EqualsRef(ref) {
			return errors.Errorf("existing block ref mismatch: got %s, want %s", actual.MarshalString(), refStr)
		}
	} else {
		var found bool
		// Read raw (possibly compressed) data from source.
		data, found, err = src.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "get block: %s", refStr)
		}
		if !found {
			return errors.Wrapf(block.ErrNotFound, "block: %s", refStr)
		}

		// Write raw data to dest and require content identity.
		if _, _, err := dest.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
			return errors.Wrapf(err, "put block: %s", refStr)
		}
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

	return followBlockGraphWithTransform(ctx, blk, src, dest, xfrm, visited)
}

// followBlockGraphWithTransform follows refs on a block or sub-block and then
// descends through any nested sub-blocks. UnixFS directory children sit behind
// FSNode -> DirentSlice -> Dirent -> NodeRef, so checking only one sub-block
// level drops file nodes from deployed manifests.
func followBlockGraphWithTransform(
	ctx context.Context,
	blk any,
	src, dest block.StoreOps,
	xfrm block.Transformer,
	visited map[string]bool,
) error {
	if withRefs, ok := blk.(block.BlockWithRefs); ok {
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
	}

	if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
		for _, sub := range withSubBlocks.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			if err := followBlockGraphWithTransform(ctx, sub, src, dest, xfrm, visited); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateBlockResponseRef requires the response to identify the requested block exactly.
func validateBlockResponseRef(want, got *block.BlockRef) error {
	if got == nil || !got.EqualsRef(want) {
		gotString := "<nil>"
		if got != nil {
			gotString = got.MarshalString()
		}
		return errors.Errorf("block response ref mismatch: got %s, want %s", gotString, want.MarshalString())
	}
	return nil
}

// streamStoreOps implements block.StoreOps by requesting blocks over the stream.
type streamStoreOps struct {
	strm s4wave_space.SRPCSpaceResourceService_DeployManifestsStream
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
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	err := s.strm.Send(&s4wave_deploy.DeployManifestsMessage{
		Body: &s4wave_deploy.DeployManifestsMessage_BlockRequest{
			BlockRequest: &s4wave_deploy.BlockRequest{
				Ref: ref,
			},
		},
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "send block request")
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	msg, err := s.strm.Recv()
	if err != nil {
		return nil, false, errors.Wrap(err, "recv block response")
	}
	resp := msg.GetBlockResponse()
	if resp == nil {
		return nil, false, errors.New("expected BlockResponse")
	}
	if err := validateBlockResponseRef(ref, resp.GetRef()); err != nil {
		return nil, false, err
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

// BeginReadOperation returns the stream source as the scoped read handle.
func (s *streamStoreOps) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
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

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil (unsupported on stream source).
func (s *streamStoreOps) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

// RmBlock is not used on the source side.
func (s *streamStoreOps) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync reports always-durable: the stream source holds no buffered writes.
func (s *streamStoreOps) Sync(_ context.Context) (bool, error) {
	return true, nil
}

// _ is a type assertion
var _ block.StoreOps = (*streamStoreOps)(nil)
