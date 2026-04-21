package core

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	hydracore "github.com/s4wave/spacewave/db/core"
	hydra_all "github.com/s4wave/spacewave/db/core/all"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/sirupsen/logrus"
)

// NewCoreBus constructs a standard in-memory bus stack with the controllers.
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
	hydra_all.AddFactories(b, sr)
	sr.AddFactory(execution_controller.NewFactory(b))
	sr.AddFactory(pass_controller.NewFactory(b))
	sr.AddFactory(task_controller.NewFactory(b))
	sr.AddFactory(worker_controller.NewFactory(b))
	sr.AddFactory(cluster_controller.NewFactory(b))
}
