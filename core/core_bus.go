package core

import (
	"context"

	bifrostcore "github.com/aperturerobotics/bifrost/core"
	nctr "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	egc "github.com/aperturerobotics/entitygraph/controller"
	lookup_concurrent "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	bucket_setup "github.com/aperturerobotics/hydra/bucket/setup"
	"github.com/aperturerobotics/hydra/dex/psecho"
	hydraeg "github.com/aperturerobotics/hydra/entitygraph"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	volume_block "github.com/aperturerobotics/hydra/volume/block"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	volume_world "github.com/aperturerobotics/hydra/volume/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with Hydra controllers.
func NewCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	opts ...cbc.Option,
) (bus.Bus, *static.Resolver, error) {
	b, sr, err := cbc.NewCoreBus(ctx, le, opts...)
	if err != nil {
		return nil, nil, err
	}

	AddFactories(b, sr)
	return b, sr, nil
}

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	addNativeFactories(b, sr)
	bifrostcore.AddFactories(b, sr)

	sr.AddFactory(nctr.NewFactory())
	sr.AddFactory(bucket_setup.NewFactory(b))

	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))

	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
	sr.AddFactory(volume_block.NewFactory(b))
	sr.AddFactory(volume_world.NewFactory(b))

	sr.AddFactory(psecho.NewFactory(b))

	sr.AddFactory(world_block_engine.NewFactory(b))

	sr.AddFactory(egc.NewFactory(b))
	sr.AddFactory(hydraeg.NewFactory(b))
}
