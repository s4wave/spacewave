package core

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_overlay "github.com/s4wave/spacewave/db/block/store/overlay"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	"github.com/s4wave/spacewave/db/dex/psecho"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	bifrostcore "github.com/s4wave/spacewave/net/core"
	nctr "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with basic Hydra controllers.
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

	sr.AddFactory(nctr.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))

	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))

	sr.AddFactory(volume_kvtxinmem.NewFactory(b))

	sr.AddFactory(block_store_inmem.NewFactory(b))
	sr.AddFactory(block_store_overlay.NewFactory(b))

	sr.AddFactory(psecho.NewFactory(b))
}
