package core

import (
	"context"

	cresolve "github.com/aperturerobotics/bldr/assembly/bridge/cresolve"
	cvolume "github.com/aperturerobotics/bldr/assembly/bridge/volume"
	assembly_controller "github.com/aperturerobotics/bldr/assembly/controller"
	plugin_fetch "github.com/aperturerobotics/bldr/plugin/host/fetch"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	hydracore "github.com/aperturerobotics/hydra/core"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus with the controllers.
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
	hydracore.AddFactories(b, sr)

	// assembly controller
	sr.AddFactory(assembly_controller.NewFactory(b))
	sr.AddFactory(cresolve.NewFactory(b))
	sr.AddFactory(cvolume.NewFactory(b))
	sr.AddFactory(plugin_fetch.NewFactory(b))
}
