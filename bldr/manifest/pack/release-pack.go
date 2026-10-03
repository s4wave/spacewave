package bldr_manifest_pack

import (
	"context"
	"io"
	"slices"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ReleaseSource is a built Manifest to encode into a release pack.
type ReleaseSource struct {
	// Access reads the Manifest.
	Access world.AccessWorldStateFunc
	// Ref references the Manifest in Access.
	Ref *bucket.ObjectRef
}

// ReleasePackConfig configures the production of a release-encoded pack.
type ReleasePackConfig struct {
	// WorldState receives the encoded blocks and the manifest bundle.
	WorldState world.WorldState
	// Sender is the world operation sender.
	Sender peer.ID
	// BundleKey is the object key of the manifest bundle.
	BundleKey string
	// Sources are the Manifests to pack.
	Sources []ReleaseSource
	// GitSHA is the source revision identity.
	GitSHA string
	// ProducerTarget is the workflow or bldr target name.
	ProducerTarget string
	// CacheSchema is the cache/artifact identity schema.
	CacheSchema string
	// Writer receives the kvfile pack bytes.
	Writer io.Writer
}

// ProduceReleasePack encodes each source Manifest with the release policy and
// writes one pack holding every reachable block. The blocks of a file tree are
// a function of its content alone, so a Manifest that did not change since the
// last release shares every block with it, and its encoded reference equals the
// published one. It returns the pack metadata and the encoded Manifests.
func ProduceReleasePack(
	ctx context.Context,
	le *logrus.Entry,
	conf *ReleasePackConfig,
) (*ManifestPackMetadata, []*bldr_manifest.ManifestRef, error) {
	// Require at least one Manifest to pack.
	if len(conf.Sources) == 0 {
		return nil, nil, errors.New("release pack has no sources")
	}

	// Encode each source Manifest through a stage held until the bundle adopts
	// it, and record its tuple.
	stage, err := conf.WorldState.StageWorldState(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer stage.Release()
	var buildType string
	tuples := make([]*ManifestTuple, 0, len(conf.Sources))
	refs := make([]*bldr_manifest.ManifestRef, 0, len(conf.Sources))
	for i, src := range conf.Sources {
		meta, err := readSourceMeta(ctx, le, src)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "read source %d", i)
		}
		if i == 0 {
			buildType = meta.GetBuildType()
		} else if meta.GetBuildType() != buildType {
			return nil, nil, errors.Errorf("source %d build type %q differs from %q", i, meta.GetBuildType(), buildType)
		}
		_, ref, err := bldr_manifest_world.EncodeReleaseManifest(ctx, le, src.Access, src.Ref, meta.CloneVT(), stage)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "encode source %d", i)
		}
		refs = append(refs, bldr_manifest.NewManifestRef(meta.CloneVT(), ref))
		tuples = append(tuples, &ManifestTuple{
			ManifestId: meta.GetManifestId(),
			PlatformId: meta.GetPlatformId(),
			Rev:        meta.GetRev(),
			ObjectKey:  conf.BundleKey,
		})
	}

	// Order the Manifests as the bundle does, by entry key, so the tuples line up
	// with the bundle entries whatever order the sources arrived in.
	order := make([]int, len(refs))
	keys := make([]string, len(refs))
	for i, ref := range refs {
		order[i] = i
		key, err := bldr_manifest.NewManifestBundleEntryKey(conf.BundleKey, ref.GetMeta())
		if err != nil {
			return nil, nil, err
		}
		keys[i] = key
	}
	slices.SortFunc(order, func(a, b int) int { return strings.Compare(keys[a], keys[b]) })
	sortedTuples := make([]*ManifestTuple, len(order))
	sortedRefs := make([]*bldr_manifest.ManifestRef, len(order))
	for i, idx := range order {
		sortedTuples[i] = tuples[idx]
		sortedRefs[i] = refs[idx]
	}
	tuples, refs = sortedTuples, sortedRefs

	// Store the encoded Manifests under one bundle with the release timestamp.
	_, bundleRef, err := StoreManifestBundles(
		ctx,
		conf.WorldState,
		conf.Sender,
		conf.BundleKey,
		nil,
		refs,
		bldr_manifest_world.ReleaseManifestTime,
	)
	if err != nil {
		return nil, nil, err
	}

	// Write the bundle's blocks as one kvfile pack.
	entry, packSHA, err := PackManifestBundle(ctx, conf.WorldState, conf.ProducerTarget, bundleRef, conf.Writer)
	if err != nil {
		return nil, nil, err
	}

	// Describe the pack for the consumer.
	meta, err := NewMetadata(conf.GitSHA, buildType, conf.ProducerTarget, false, conf.CacheSchema, tuples, bundleRef, entry, packSHA)
	if err != nil {
		return nil, nil, err
	}
	return meta, refs, nil
}

// readSourceMeta reads the metadata of a source Manifest.
func readSourceMeta(ctx context.Context, le *logrus.Entry, src ReleaseSource) (*bldr_manifest.ManifestMeta, error) {
	var meta *bldr_manifest.ManifestMeta
	err := bldr_manifest_world.AccessManifest(ctx, le, src.Access, src.Ref, func(
		_ context.Context,
		_ *bucket_lookup.Cursor,
		_ *block.Cursor,
		manifest *bldr_manifest.Manifest,
		_ *unixfs.FSHandle,
		_ *unixfs.FSHandle,
	) error {
		meta = manifest.GetMeta().CloneVT()
		return nil
	})
	return meta, err
}
