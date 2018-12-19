package core

import (
	"context"

	nctr "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/aperturerobotics/hydra/volume/badger"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with Bifrost controllers.
func NewCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	builtInFactories ...controller.Factory,
) (bus.Bus, *static.Resolver, error) {
	b, sr, err := cbc.NewCoreBus(ctx, le, builtInFactories...)
	if err != nil {
		return nil, nil, err
	}

	sr.AddFactory(nctr.NewFactory(b))
	sr.AddFactory(egc.NewFactory(b))
	sr.AddFactory(volume_badger.NewFactory(b))

	return b, sr, nil
}
