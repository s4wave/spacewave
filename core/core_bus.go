package core

import (
	"context"

	bifrostcore "github.com/aperturerobotics/bifrost/core"
	nctr "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	hydraeg "github.com/aperturerobotics/hydra/entitygraph"
	"github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with Hydra controllers.
func NewCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	builtInFactories ...controller.Factory,
) (bus.Bus, *static.Resolver, error) {
	b, sr, err := cbc.NewCoreBus(ctx, le, builtInFactories...)
	if err != nil {
		return nil, nil, err
	}

	sr.AddFactory(nctr.NewFactory())
	sr.AddFactory(egc.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
	sr.AddFactory(hydraeg.NewFactory(b))
	bifrostcore.AddFactories(b, sr)

	addNativeFactories(b, sr)
	return b, sr, nil
}
