//+build !js

package main

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	badger "github.com/aperturerobotics/hydra/volume/badger"
	"github.com/sirupsen/logrus"
)

func addStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
) (directive.AttachedValue, directive.Reference, error) {
	sr.AddFactory(badger.NewFactory(b))
	return bus.ExecOneOff(ctx, b, resolver.NewLoadControllerWithConfig(&badger.Config{
		Dir:     "data",
		Verbose: true,
	}), nil)
}
