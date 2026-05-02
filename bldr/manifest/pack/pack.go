package bldr_manifest_pack

import (
	"context"
	"io"
	"sort"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/identity"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
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
	if err := ValidateCleanObjectRef("manifest_bundle_ref", bundleRef); err != nil {
		return nil, nil, err
	}
	var blocks []packBlock
	seen := make(map[string]struct{})
	appendBlocks := func(rootRef *bucket.ObjectRef, ctor func() block.Block) error {
		if err := ValidateCleanObjectRef("manifest_pack_root", rootRef); err != nil {
			return err
		}
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
					key := entry.Ref.MarshalString()
					if _, ok := seen[key]; ok {
						return true, nil
					}
					seen[key] = struct{}{}
					blocks = append(blocks, packBlock{
						key:  key,
						hash: entry.Ref.GetHash().Clone(),
						data: append([]byte(nil), entry.Data...),
					})
					return true, nil
				},
				bls.GetBucket(),
				readXfrm,
				1,
				true,
			)
		})
	}
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
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].key < blocks[j].key
	})
	idx := 0
	res, err := writer.PackBlocks(w, func() (*hash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		block := blocks[idx]
		idx++
		return block.hash, block.data, nil
	})
	if err != nil {
		return nil, nil, err
	}
	if res.BlockCount == 0 {
		return nil, nil, errors.New("manifest bundle pack contains no blocks")
	}
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
	meta := &ManifestPackMetadata{
		FormatVersion:     MetadataFormatVersion,
		GitSha:            gitSHA,
		BuildType:         buildType,
		ProducerTarget:    producerTarget,
		ReactDev:          reactDev,
		CacheSchema:       cacheSchema,
		ManifestBundleRef: bundleRef.Clone(),
		Pack:              entry.CloneVT(),
		PackSha256:        append([]byte(nil), packSHA256...),
	}
	if len(tuples) != 0 {
		meta.Manifests = make([]*ManifestTuple, len(tuples))
		for i, tuple := range tuples {
			meta.Manifests[i] = tuple.CloneVT()
		}
	}
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	return meta, nil
}

type packBlock struct {
	key  string
	hash *hash.Hash
	data []byte
}
