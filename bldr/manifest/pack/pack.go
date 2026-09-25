package bldr_manifest_pack

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/order"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
)

// PackManifestBundle writes all blocks reachable from a manifest bundle ref.
func PackManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	resourceID string,
	bundleRef *bucket.ObjectRef,
	w io.Writer,
) (*packfile.PackfileEntry, []byte, error) {
	// Pack only standalone references with fully specified source ownership.
	if err := ValidateCleanObjectRef("manifest_bundle_ref", bundleRef); err != nil {
		return nil, nil, err
	}

	// Retain structural edges with the encoded bytes. The walker may
	// visit siblings before descendants; physical output follows each subtree.
	blocks := make(map[string]packBlock)
	graph := order.NewGraph()
	var roots []*block.BlockRef
	appendBlocks := func(rootRef *bucket.ObjectRef, ctor func() block.Block) error {
		if err := ValidateCleanObjectRef("manifest_pack_root", rootRef); err != nil {
			return err
		}
		roots = append(roots, rootRef.GetRootRef())
		return ws.AccessWorldState(ctx, rootRef, func(bls *bucket_lookup.Cursor) error {
			readXfrm := bls.GetTransformer()
			if readXfrm == nil {
				readXfrm = block_transform.NewTransformerWithSteps(nil)
			}
			return bucket_lookup.WalkObjectBlocks(
				ctx,
				bucket_lookup.NewWalkObjectBlocksWithRef(rootRef.GetRootRef(), ctor),
				func(entry *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
					if entry.Err != nil {
						return false, entry.Err
					}
					if entry.Ref == nil || entry.Ref.GetEmpty() || !entry.Found || entry.IsSubBlock || len(entry.Data) == 0 {
						return true, nil
					}

					// Capture each encoded payload and its structural edges once.
					key := entry.Ref.GetHash().MarshalString()
					if _, ok := blocks[key]; ok {
						return true, nil
					}
					children, err := block.ExtractBlockRefs(entry.Blk)
					if err != nil {
						return false, err
					}
					graph.Add(entry.Ref, children)
					blocks[key] = packBlock{
						hash:   entry.Ref.GetHash().CloneVT(),
						stored: &block.StoredBlock{Data: bytes.Clone(entry.Data), Refs: children, RefsKnown: true},
					}
					return true, nil
				},
				bls.GetBucket(),
				readXfrm,
				1,
				true,
			)
		})
	}

	// Include the bundle and every manifest filesystem reachable through it.
	if err := appendBlocks(bundleRef, bldr_manifest.NewManifestBundleBlock); err != nil {
		return nil, nil, err
	}
	bundle, err := readManifestBundle(ctx, ws, bundleRef)
	if err != nil {
		return nil, nil, err
	}
	for i, manifestRef := range bundle.GetManifestRefs() {
		if err := appendBlocks(manifestRef.GetManifestRef(), bldr_manifest.NewManifestBlock); err != nil {
			return nil, nil, errors.Wrapf(err, "manifest_refs[%d]", i)
		}
	}

	// Order before writing so related values remain adjacent in the pack.
	refs, err := graph.Order(ctx, roots)
	if err != nil {
		return nil, nil, err
	}
	idx := 0
	res, err := writer.PackBlocks(w, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(refs) {
			return nil, nil, nil
		}
		blk := blocks[refs[idx].GetHash().MarshalString()]
		idx++
		return blk.hash, blk.stored, nil
	})
	if err != nil {
		return nil, nil, err
	}
	if res.BlockCount == 0 {
		return nil, nil, errors.New("manifest bundle pack contains no blocks")
	}

	// Derive immutable identity and metadata from the encoded physical bytes.
	packID, err := identity.BuildPackID(resourceID, res)
	if err != nil {
		return nil, nil, err
	}
	entry := &packfile.PackfileEntry{
		Id:                 packID,
		BloomFilter:        res.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         res.BlockCount,
		SizeBytes:          res.BytesWritten,
		CreatedAt:          timestamppb.Now(),
	}
	return entry, res.PackBytesDigest, nil
}

// NewMetadata constructs validated metadata for one manifest-pack artifact.
func NewMetadata(
	gitSHA string,
	buildType string,
	producerTarget string,
	reactDev bool,
	cacheSchema string,
	tuples []*ManifestTuple,
	bundleRef *bucket.ObjectRef,
	entry *packfile.PackfileEntry,
	packSHA256 []byte,
) (*ManifestPackMetadata, error) {
	// Own copies of all mutable artifact and manifest metadata.
	meta := &ManifestPackMetadata{
		FormatVersion:     MetadataFormatVersion,
		GitSha:            gitSHA,
		BuildType:         buildType,
		ProducerTarget:    producerTarget,
		ReactDev:          reactDev,
		CacheSchema:       cacheSchema,
		ManifestBundleRef: bundleRef.CloneVT(),
		Pack:              entry.CloneVT(),
		PackSha256:        bytes.Clone(packSHA256),
	}
	if len(tuples) != 0 {
		meta.Manifests = make([]*ManifestTuple, len(tuples))
		for i, tuple := range tuples {
			meta.Manifests[i] = tuple.CloneVT()
		}
	}

	// Return only metadata that satisfies the import contract.
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	return meta, nil
}

// packBlock is one block queued for the pack writer.
type packBlock struct {
	// hash identifies the encoded payload.
	hash *hash.Hash
	// stored is the encoded payload copied from the source store, with the
	// refs its block holds.
	stored *block.StoredBlock
}
