package bldr_manifest_pack

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// ProducerConfig configures manifest-pack production for one tuple.
type ProducerConfig struct {
	// Bus resolves FetchManifest directives.
	Bus bus.Bus
	// WorldState stores the fetched manifest bundle before packing.
	WorldState world.WorldState
	// Sender is the world operation sender.
	Sender peer.ID
	// Tuple is the single manifest tuple to produce.
	Tuple *ManifestTuple
	// BuildType is the FetchManifest build type.
	BuildType string
	// GitSHA is the source revision identity.
	GitSHA string
	// ProducerTarget is the workflow or bldr target name.
	ProducerTarget string
	// ReactDev indicates the producer used react_dev mode.
	ReactDev bool
	// CacheSchema is the cache/artifact identity schema.
	CacheSchema string
	// Writer receives the kvfile pack bytes.
	Writer io.Writer
	// DistDir optionally replaces the built manifest contents with a
	// directory packaged after the build, such as a signed app bundle. The
	// manifest keeps the built identity and revision.
	DistDir string
	// Entrypoint is the path within DistDir to the entrypoint. Required
	// when DistDir is set.
	Entrypoint string
}

// ProduceManifestPack resolves one tuple and writes its manifest-pack artifact.
func ProduceManifestPack(ctx context.Context, conf *ProducerConfig) (*ManifestPackMetadata, error) {
	// Validate the producer config and resolve the manifest tuple.
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	manifestRef, err := ResolveManifestTuple(ctx, conf.Bus, conf.Tuple, conf.BuildType)
	if err != nil {
		return nil, errors.Wrap(err, "resolve manifest tuple")
	}

	// Replace the built manifest with the dist-dir artifact when configured.
	if conf.DistDir != "" {
		manifestRef, err = commitDistDirManifest(ctx, conf, manifestRef.GetMeta())
		if err != nil {
			return nil, err
		}
	}

	// Store the manifest bundle at the resolved revision.
	tuple := conf.Tuple.CloneVT()
	tuple.Rev = manifestRef.GetMeta().GetRev()
	_, bundleRef, err := StoreManifestBundle(
		ctx,
		conf.WorldState,
		conf.Sender,
		tuple,
		manifestRef,
		nil,
	)
	if err != nil {
		return nil, err
	}

	// Pack the bundle bytes and build the pack metadata.
	entry, packSHA, err := PackManifestBundle(
		ctx,
		conf.WorldState,
		conf.ProducerTarget,
		bundleRef,
		conf.Writer,
	)
	if err != nil {
		return nil, err
	}
	return NewMetadata(
		conf.GitSHA,
		conf.BuildType,
		conf.ProducerTarget,
		conf.ReactDev,
		conf.CacheSchema,
		[]*ManifestTuple{tuple},
		bundleRef,
		entry,
		packSHA,
	)
}

// Validate validates the producer config.
func (c *ProducerConfig) Validate() error {
	// Require the core collaborators.
	if c == nil {
		return errors.New("producer config is nil")
	}
	if c.Bus == nil {
		return errors.New("bus is nil")
	}
	if c.WorldState == nil {
		return errors.New("world state is nil")
	}

	// Require a valid tuple and identity fields.
	if err := c.Tuple.ValidateRequest(); err != nil {
		return err
	}
	if c.BuildType == "" {
		return errors.New("build_type is empty")
	}
	if c.GitSHA == "" {
		return errors.New("git_sha is empty")
	}
	if c.ProducerTarget == "" {
		return errors.New("producer_target is empty")
	}
	if c.CacheSchema == "" {
		return errors.New("cache_schema is empty")
	}
	if c.Writer == nil {
		return errors.New("writer is nil")
	}

	// Require the dist dir and entrypoint together.
	if (c.DistDir == "") != (c.Entrypoint == "") {
		return errors.New("dist_dir and entrypoint must be set together")
	}
	return nil
}

// commitDistDirManifest writes a manifest with the built metadata whose dist
// tree is conf.DistDir. The directory is the complete artifact, so the
// manifest carries no assets.
func commitDistDirManifest(
	ctx context.Context,
	conf *ProducerConfig,
	meta *bldr_manifest.ManifestMeta,
) (*bldr_manifest.ManifestRef, error) {
	// Stat the entrypoint within the dist directory.
	entrypointPath := filepath.Join(conf.DistDir, filepath.FromSlash(conf.Entrypoint))
	if _, err := os.Stat(entrypointPath); err != nil {
		return nil, errors.Wrap(err, "stat dist dir entrypoint")
	}

	// Commit the manifest whose dist tree is the dist directory.
	manifest := bldr_manifest.NewManifest(meta.CloneVT(), conf.Entrypoint)
	distFs := osfs.New(conf.DistDir, osfs.WithBoundOS())
	ref, err := world.AccessObject(ctx, conf.WorldState.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifest, distFs, nil, timestamppb.Now())
	})
	if err != nil {
		return nil, errors.Wrap(err, "commit dist dir manifest")
	}
	return bldr_manifest.NewManifestRef(manifest.GetMeta(), ref), nil
}
