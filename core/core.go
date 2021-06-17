package core

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	execution_controller "github.com/aperturerobotics/forge/execution/controller"
	hydracore "github.com/aperturerobotics/hydra/core"
	hydra_all "github.com/aperturerobotics/hydra/core/all"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with the controllers.
func NewCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	builtInFactories ...controller.Factory,
) (bus.Bus, *static.Resolver, error) {
	b, sr, err := cbc.NewCoreBus(ctx, le, builtInFactories...)
	if err != nil {
		return nil, nil, err
	}

	AddFactories(b, sr)
	return b, sr, nil
}

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	hydracore.AddFactories(b, sr)
	hydra_all.AddFactories(b, sr)
	sr.AddFactory(execution_controller.NewFactory(b))
}
