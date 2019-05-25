package core_test

import (
	"context"

	nctr "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/sirupsen/logrus"
)

// NewTestingBus constructs a minimal in-memory Hydra bus stack.
func NewTestingBus(
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

	return b, sr, nil
}
