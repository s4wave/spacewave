//go:build !js && !redis
// +build !js,!redis

package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	"github.com/sirupsen/logrus"
)

func AddStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
	verbose bool,
) (controller.Controller, directive.Instance, directive.Reference, error) {
	sr.AddFactory(volume_bolt.NewFactory(b))
	return loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{
			Path:    "data",
			Verbose: verbose,
		}),
		nil,
	)
}
