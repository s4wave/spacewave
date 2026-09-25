package publisher

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
)

// Publish uploads a committed release World, then advances its public root.
// Every referenced block must exist before any network write begins. Failed
// uploads leave the previous root intact; content-addressed packs are retryable.
// The caller must exclusively own the local World for the operation's lifetime.
func Publish(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata, opts cdn_publish.Options) (*sobject.SORoot, error) {
	// Require explicit cloud authority and a complete release before exporting.
	if eng == nil || opts.Client == nil || opts.DstSpaceID == "" || opts.ValidatorKeyPem == "" || opts.CdnBaseURL == "" {
		return nil, errors.New("release World, session, destination Space, signer, and CDN URL are required")
	}
	if err := metadata.Validate(); err != nil {
		return nil, err
	}
	head, blocks, err := collectBlocks(ctx, eng, metadata)
	if err != nil {
		return nil, err
	}
	if opts.Logger != nil {
		opts.Logger.WithField("blocks", len(blocks)).Info("verified release content")
	}
	entries, err := cdn_publish.FetchPackEntries(ctx, opts.Client, opts.DstSpaceID)
	if err != nil {
		return nil, err
	}
	// Exact pack indexes prove presence; Bloom filters only narrow the search.
	remote := packfile_store.NewPackfileStore(cdn_bstore.NewAnonymousOpener(nil, opts.CdnBaseURL, opts.DstSpaceID), nil)
	defer remote.Close()
	remote.UpdateManifest(entries)
	refs := make([]*block.BlockRef, len(blocks))
	for i, entry := range blocks {
		refs[i] = entry.ref
	}
	exists, err := remote.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, errors.Wrap(err, "check published release blocks")
	}
	missing := blocks[:0]
	var missingBytes uint64
	for i, entry := range blocks {
		if !exists[i] {
			missing = append(missing, entry)
			missingBytes += uint64(len(entry.stored.Data))
		}
	}
	blocks = missing
	if opts.Logger != nil {
		opts.Logger.WithField("blocks", len(blocks)).WithField("bytes", missingBytes).Info("publishing missing release content")
	}

	// Use the standard pack writer and resource-scoped content identity.
	index := 0
	_, err = delta.EmitDeltaChunks(ctx, opts.DstSpaceID, func() (*hash.Hash, *block.StoredBlock, error) {
		if index == len(blocks) {
			return nil, nil, nil
		}
		entry := blocks[index]
		index++
		return entry.ref.GetHash(), entry.stored, nil
	}, delta.DefaultMaxChunkBytes, func(ctx context.Context, chunk int, entry *packfile.PackfileEntry, data []byte) error {
		if opts.Logger != nil {
			opts.Logger.WithField("pack", chunk).WithField("bytes", len(data)).Info("uploading release content")
		}
		return cdn_publish.PushPackData(ctx, opts, data, entry.GetBloomFilter())
	})
	if err != nil {
		return nil, errors.Wrap(err, "upload release packs")
	}

	// The signed root is the sole publication point after all packs are durable.
	return cdn_publish.PostRoot(ctx, opts, head)
}

// packedBlock retains a content-addressed block from the verified local
// closure.
type packedBlock struct {
	// ref identifies the bytes as stored, before read transformations.
	ref *block.BlockRef
	// stored holds the immutable block payload with its recorded refs.
	stored *block.StoredBlock
}

// walkedBlock identifies a decoded subtree within its storage context.
type walkedBlock struct {
	// bucketID selects the storage used by child references.
	bucketID string
	// transform identifies the decoder configuration applied to stored bytes.
	transform string
	// ref identifies the immutable stored block.
	ref string
	// blockType distinguishes different reference layouts over the same bytes.
	blockType reflect.Type
}

// collectBlocks checks the World, object roots, and manifest filesystem closures.
// Manifest refs can cross object boundaries and must be walked explicitly.
func collectBlocks(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata) (*bucket.ObjectRef, []packedBlock, error) {
	// Collect object roots from one committed read transaction.
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()
	roots := map[string]*bucket.ObjectRef{}
	ctors := map[string]block.Ctor{}
	objects := tx.IterateObjects(ctx, "", false)
	defer objects.Close()
	if err := objects.Seek(""); err != nil {
		return nil, nil, err
	}
	for objects.Valid() {
		object, exists, err := tx.GetObject(ctx, objects.Key())
		if err != nil {
			world.ReleaseObjectState(object)
			return nil, nil, err
		}
		if !exists {
			return nil, nil, errors.New("release object disappeared during export")
		}
		ref, _, err := object.GetRootRef(ctx)
		world.ReleaseObjectState(object)
		if err != nil {
			return nil, nil, err
		}
		if ref != nil && ref.GetRootRef() != nil {
			key := ref.MarshalString()
			roots[key] = ref
			typeID, err := world_types.GetObjectType(ctx, tx, objects.Key())
			if err != nil {
				return nil, nil, err
			}
			switch {
			case typeID == manifest_world.ManifestTypeID:
				ctors[key] = bldr_manifest.NewManifestBlock
			case objects.Key() == ChannelDirectoryKey:
				ctors[key] = func() block.Block { return &release.ChannelDirectory{} }
			case strings.HasPrefix(objects.Key(), ChannelDirectoryKey+"/"):
				ctors[key] = func() block.Block { return &release.ReleaseMetadata{} }
			}
		}
		if !objects.Next() {
			break
		}
	}
	if err := objects.Err(); err != nil {
		return nil, nil, err
	}

	// Selected manifests precede historical objects so their file closures stay
	// together in the emitted packs. Metadata order is stable across retries.
	var rootKeys []string
	for _, manifest := range metadata.GetManifestRefs() {
		ref := manifest.GetManifestRef()
		key := ref.MarshalString()
		roots[key], ctors[key] = ref, bldr_manifest.NewManifestBlock
		if !slices.Contains(rootKeys, key) {
			rootKeys = append(rootKeys, key)
		}
	}

	// Walk through the owning cursor so bucket and transform references retain
	// their original meaning. Single-worker traversal owns the collected map.
	var head *bucket.ObjectRef
	blocks := map[string]struct{}{}
	walked := map[walkedBlock]struct{}{}
	var result []packedBlock
	err = eng.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		head = cursor.GetRefWithOpArgs()
		roots[head.MarshalString()] = head
		ctors[head.MarshalString()] = func() block.Block { return &world_block.World{} }
		for _, key := range slices.Sorted(maps.Keys(roots)) {
			if !slices.Contains(rootKeys, key) {
				rootKeys = append(rootKeys, key)
			}
		}

		// Each object retains the owning bucket and transform during traversal.
		for _, key := range rootKeys {
			ref := roots[key]
			walk, err := cursor.FollowRef(ctx, ref)
			if err != nil {
				return err
			}
			transformData, err := walk.GetTransformConf().MarshalVT()
			if err != nil {
				walk.Release()
				return err
			}
			transform := string(transformData)
			bucketID := walk.GetOpArgs().GetBucketId()
			transformer := walk.GetTransformer()
			if transformer == nil {
				transformer = block_transform.NewTransformerWithSteps(nil)
			}
			err = bucket_lookup.WalkObjectBlocks(ctx,
				bucket_lookup.NewWalkObjectBlocksWithRef(ref.GetRootRef(), ctors[key]),
				func(entry *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
					if entry.Err != nil {
						return false, entry.Err
					}
					if entry.IsSubBlock {
						return true, nil
					}
					if entry.Ref.GetEmpty() {
						return true, nil
					}
					if !entry.Found {
						return false, errors.Errorf("release block %s is missing", entry.Ref.MarshalString())
					}
					if err := entry.Ref.VerifyData(entry.Data, true); err != nil {
						return false, err
					}
					key := entry.Ref.MarshalString()
					if _, exists := blocks[key]; !exists {
						stored, err := walk.GetBucket().GetStoredBlock(ctx, entry.Ref)
						if err != nil {
							return false, err
						}
						if !stored.GetRefsKnown() {
							return false, errors.Wrap(block.ErrRefsUnknown, key)
						}
						blocks[key] = struct{}{}
						result = append(result, packedBlock{ref: entry.Ref.CloneVT(), stored: stored})
					}
					// Reused filesystem trees need one complete traversal. Keep the
					// decoder and storage context in the key so a differently typed
					// or transformed root still exposes all of its references.
					visit := walkedBlock{bucketID: bucketID, transform: transform, ref: key, blockType: reflect.TypeOf(entry.Blk)}
					if _, seen := walked[visit]; seen {
						return false, nil
					}
					walked[visit] = struct{}{}
					return true, nil
				}, walk.GetBucket(), transformer, 1, true)
			walk.Release()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if head == nil || head.GetEmpty() || len(blocks) == 0 {
		return nil, nil, errors.New("release World has no content")
	}

	// The single-worker walk orders children by reference ID. Preserve that
	// locality rather than scattering adjacent file blocks by content hash.
	return head, result, nil
}
