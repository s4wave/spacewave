//+build js

package main

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	vidb "github.com/aperturerobotics/hydra/volume/js/indexeddb"
	"github.com/sirupsen/logrus"
)

func addStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
) (directive.AttachedValue, directive.Reference, error) {
	sr.AddFactory(vidb.NewFactory(b))
	return bus.ExecOneOff(ctx, b, resolver.NewLoadControllerWithConfig(&vidb.Config{
		DatabaseName: "example",
		Verbose:      true,
	}), nil)
}
