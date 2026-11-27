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
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
)

// releaseClosure is the verified closure of a release World without block
// data: it keeps each block's ref and refs, so its size is a small fraction
// of the content's. read loads a block's data again from the World when a
// pack needs it.
type releaseClosure struct {
	// head is the World root the closure was walked from.
	head *bucket.ObjectRef
	// blocks lists each block once, in walk order.
	blocks []packedBlock
	// index maps a block's pack key to its position in blocks.
	index map[string]int
	// sources holds one root per bucket that stores closure blocks; a
	// block's source indexes it.
	sources []*bucket.ObjectRef
}

// loadBlock reads one closure block's data back from the World, verified
// against its ref, with the refs the walk recorded.
type loadBlock func(ctx context.Context, entry *packedBlock) (*block.StoredBlock, error)

// read runs fn with a loader of closure blocks from eng, verified against
// their refs, valid until fn returns. The loader is not safe for concurrent
// use. The caller still exclusively owns the World, as for the walk.
func (c *releaseClosure) read(ctx context.Context, eng world.Engine, fn func(read loadBlock) error) error {
	return eng.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Follow each source root once, on its first block.
		buckets := make([]*bucket_lookup.Cursor, len(c.sources))
		defer func() {
			for _, b := range buckets {
				if b != nil {
					b.Release()
				}
			}
		}()

		// Read the stored bytes, check them, and attach the walked refs.
		return fn(func(ctx context.Context, entry *packedBlock) (*block.StoredBlock, error) {
			// Open the block's source bucket on first use.
			if buckets[entry.source] == nil {
				b, err := cursor.FollowRef(ctx, c.sources[entry.source])
				if err != nil {
					return nil, err
				}
				buckets[entry.source] = b
			}
			stored, err := buckets[entry.source].GetStoredBlock(ctx, entry.ref)
			if err != nil {
				return nil, err
			}
			if stored == nil {
				return nil, errors.Errorf("release block %s is missing", entry.ref.MarshalString())
			}
			if err := entry.ref.VerifyData(stored.Data, true); err != nil {
				return nil, err
			}
			return &block.StoredBlock{Data: stored.Data, Refs: entry.refs, RefsSource: block.RefsListed}, nil
		})
	})
}

// reader adapts a closure loader to a read of blocks by hash. A hash outside
// the closure reads as absent.
func (c *releaseClosure) reader(read loadBlock) cdn_publish.ReadBlock {
	return func(ctx context.Context, h *hash.Hash) (*block.StoredBlock, error) {
		i, ok := c.index[string(packfile.BlockKey(h))]
		if !ok {
			return nil, nil
		}
		return read(ctx, &c.blocks[i])
	}
}

// packedBlock is one block of the verified local closure, without its data.
type packedBlock struct {
	// ref identifies the bytes as stored, before read transformations.
	ref *block.BlockRef
	// refs are the block's outgoing refs, as stored or as the typed walk
	// recovered them.
	refs []*block.BlockRef
	// source indexes the releaseClosure source whose bucket stores it.
	source int
	// decoded reports that the walk decoded the block and visited its refs.
	// The walk stops at a block it has no decoder for, such as an old Object
	// root a changelog entry references, so the closure omits its children.
	decoded bool
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
// Manifest refs can cross object boundaries and must be walked explicitly. It
// verifies every block's data as it walks and keeps none of it.
func collectBlocks(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata) (*releaseClosure, error) {
	closure := &releaseClosure{index: map[string]int{}}
	if err := closure.collect(ctx, eng, metadata); err != nil {
		return nil, err
	}
	return closure, nil
}

// collect records the verified blocks in manifest-local walk order.
func (c *releaseClosure) collect(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata) error {
	// Collect object roots from one committed read transaction.
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Initialize lookup tables for object roots and block decoders.
	roots := map[string]*bucket.ObjectRef{}
	ctors := map[string]block.Ctor{}

	// Seek the committed object cursor to its first object.
	objects := tx.IterateObjects(ctx, "", false)
	defer objects.Close()
	if err := objects.Seek(""); err != nil {
		return err
	}

	// Collect typed roots while releasing each loaded object state.
	for objects.Valid() {
		object, exists, err := tx.GetObject(ctx, objects.Key())
		if err != nil {
			world.ReleaseObjectState(object)
			return err
		}
		if !exists {
			return errors.New("release object disappeared during export")
		}
		ref, _, err := object.GetRootRef(ctx)
		world.ReleaseObjectState(object)
		if err != nil {
			return err
		}
		if ref != nil && ref.GetRootRef() != nil {
			key := ref.MarshalString()
			roots[key] = ref
			typeID, err := world_types.GetObjectType(ctx, tx, objects.Key())
			if err != nil {
				return err
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

	// Return errors reported by the object cursor.
	if err := objects.Err(); err != nil {
		return err
	}

	// Selected manifests precede historical objects so their file closures stay
	// together in the emitted packs. Metadata order is stable across retries.
	rootKeys := make([]string, 0, len(roots)+len(metadata.GetManifestRefs()))
	selectedRoots := make(map[string]struct{}, len(metadata.GetManifestRefs()))
	for _, manifest := range metadata.GetManifestRefs() {
		ref := manifest.GetManifestRef()
		key := ref.MarshalString()
		roots[key], ctors[key] = ref, bldr_manifest.NewManifestBlock
		if _, exists := selectedRoots[key]; !exists {
			selectedRoots[key] = struct{}{}
			rootKeys = append(rootKeys, key)
		}
	}

	// Walk through the owning cursor so bucket and transform references retain
	// their original meaning. Single-worker traversal owns the collected map.
	var head *bucket.ObjectRef
	blocks := c.index
	walked := map[walkedBlock]struct{}{}
	sources := map[string]int{}
	err = eng.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Register the World root and append remaining object roots in stable order.
		head = cursor.GetRefWithOpArgs()
		roots[head.MarshalString()] = head
		ctors[head.MarshalString()] = func() block.Block { return &world_block.World{} }
		for _, key := range slices.Sorted(maps.Keys(roots)) {
			if _, selected := selectedRoots[key]; !selected {
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
			transform, err := walk.TransformKey()
			if err != nil {
				walk.Release()
				return err
			}
			bucketID := walk.GetOpArgs().GetBucketId()
			source, ok := sources[bucketID]
			if !ok {
				source = len(c.sources)
				sources[bucketID] = source
				c.sources = append(c.sources, ref)
			}
			transformer := walk.GetTransformer()
			if transformer == nil {
				transformer = block_transform.NewTransformerWithSteps(nil)
			}
			err = bucket_lookup.WalkObjectBlocks(ctx,
				bucket_lookup.NewWalkObjectBlocksWithRef(ref.GetRootRef(), ctors[key]),
				func(entry *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
					// Propagate walker errors and skip sub-blocks and empty refs.
					if entry.Err != nil {
						return false, entry.Err
					}
					if entry.IsSubBlock {
						return true, nil
					}
					if entry.Ref.GetEmpty() {
						return true, nil
					}

					// Require each nonempty release block to be present and verified.
					if !entry.Found {
						return false, errors.Errorf("release block %s is missing", entry.Ref.MarshalString())
					}
					if err := entry.Ref.VerifyData(entry.Data, true); err != nil {
						return false, err
					}

					// Add unseen verified blocks with their refs but not their
					// data, recovering unknown refs from the block the typed walk
					// decoded rather than exporting a leaf.
					packKey := string(packfile.BlockKey(entry.Ref.GetHash()))
					index, exists := blocks[packKey]
					if !exists {
						stored, err := walk.GetStoredBlock(ctx, entry.Ref)
						if err != nil {
							return false, err
						}
						refs := stored.GetRefs()
						if !stored.GetRefsSource().Recorded() {
							refs, _, err = entry.BlockRefs(ctx, walk.GetBucket())
							if err != nil {
								return false, err
							}
						}
						index = len(c.blocks)
						blocks[packKey] = index
						c.blocks = append(c.blocks, packedBlock{ref: entry.Ref.CloneVT(), refs: refs, source: source})
					}
					if entry.Blk != nil {
						c.blocks[index].decoded = true
					}
					key := entry.Ref.MarshalString()

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
		return err
	}
	if head == nil || head.GetEmpty() || len(blocks) == 0 {
		return errors.New("release World has no content")
	}

	// The single-worker walk orders children by reference ID. Preserve that
	// locality rather than scattering adjacent file blocks by content hash.
	c.head = head
	return nil
}

// missing selects absent blocks without changing their walk order or sources.
func (c *releaseClosure) missing(ctx context.Context, store block.StoreOps) (*releaseSelection, error) {
	// Ask the remote store about every block in walk order.
	refs := make([]*block.BlockRef, len(c.blocks))
	for i, entry := range c.blocks {
		refs[i] = entry.ref
	}
	exists, err := store.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, errors.Wrap(err, "check published release blocks")
	}

	// Borrow absent entries in their original walk order.
	var missing []*packedBlock
	for i := range c.blocks {
		if !exists[i] {
			missing = append(missing, &c.blocks[i])
		}
	}
	return &releaseSelection{blocks: missing}, nil
}

// count reports the number of verified blocks for publication progress.
func (c *releaseClosure) count() int {
	return len(c.blocks)
}

// emit reads the complete closure in walk order into content-addressed packs.
func (c *releaseClosure) emit(
	ctx context.Context,
	read loadBlock,
	spaceID string,
	emit delta.ChunkEmitter,
) ([]*packfile.PackfileEntry, uint64, error) {
	index := 0
	return emitBlocks(ctx, read, spaceID, func() *packedBlock {
		if index == len(c.blocks) {
			return nil
		}
		entry := &c.blocks[index]
		index++
		return entry
	}, emit)
}

// emitBlocks loads each selected block only as its standard pack fills.
func emitBlocks(
	ctx context.Context,
	read loadBlock,
	spaceID string,
	next func() *packedBlock,
	emit delta.ChunkEmitter,
) ([]*packfile.PackfileEntry, uint64, error) {
	// Adapt selected metadata to the standard verified-block pack writer.
	var size uint64
	entries, err := delta.EmitDeltaChunks(ctx, spaceID, func() (*hash.Hash, *block.StoredBlock, error) {
		// Read the next selected block and retain its verified outgoing refs.
		entry := next()
		if entry == nil {
			return nil, nil, nil
		}
		stored, err := read(ctx, entry)
		if err != nil {
			return nil, nil, err
		}
		size += uint64(len(stored.Data))
		return entry.ref.GetHash(), stored, nil
	}, delta.DefaultMaxChunkBytes, emit)
	return entries, size, err
}
