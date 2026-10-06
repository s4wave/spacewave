//go:build !js && !redis

package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// AddStorageVolume starts an s4db Volume in data.s4wave and waits for it to
// run.
func AddStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
	verbose bool,
) (controller.Controller, directive.Instance, directive.Reference, error) {
	sr.AddFactory(volume_s4db.NewFactory(b))
	return loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_s4db.Config{
			Path:    "data.s4wave",
			Verbose: verbose,
		}),
		nil,
	)
}
