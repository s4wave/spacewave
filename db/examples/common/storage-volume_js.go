//go:build js

package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	vidb "github.com/s4wave/spacewave/db/volume/js/indexeddb"
	"github.com/sirupsen/logrus"
)

func AddStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
	verbose bool,
) (controller.Controller, directive.Instance, directive.Reference, error) {
	sr.AddFactory(vidb.NewFactory(b))
	return loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&vidb.Config{
			DatabaseName: "example",
			Verbose:      verbose,
		}),
		nil,
	)
}
