package bldr_manifest_pack

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
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
}

// ProduceManifestPack resolves one tuple and writes its manifest-pack artifact.
func ProduceManifestPack(ctx context.Context, conf *ProducerConfig) (*ManifestPackMetadata, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	manifestRef, err := ResolveManifestTuple(ctx, conf.Bus, conf.Tuple, conf.BuildType)
	if err != nil {
		return nil, errors.Wrap(err, "resolve manifest tuple")
	}
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
	if c == nil {
		return errors.New("producer config is nil")
	}
	if c.Bus == nil {
		return errors.New("bus is nil")
	}
	if c.WorldState == nil {
		return errors.New("world state is nil")
	}
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
	return nil
}
