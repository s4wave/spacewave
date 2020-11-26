//+build !js,redis

package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	volume_redis "github.com/aperturerobotics/hydra/volume/redis"
	"github.com/sirupsen/logrus"
)

func AddStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
	verbose bool,
) (controller.Controller, directive.Instance, directive.Reference, error) {
	sr.AddFactory(volume_redis.NewFactory(b))
	return loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_redis.Config{
			Url:     "redis://localhost:6379",
			Verbose: verbose,
		}),
		nil,
	)
}
