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
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	"github.com/sirupsen/logrus"
)

// AddStorageVolume starts the example's browser volume and waits until its
// controller runs.
func AddStorageVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sr *static.Resolver,
	verbose bool,
) (controller.Controller, directive.Instance, directive.Reference, error) {
	sr.AddFactory(volume_browser.NewFactory(b))
	return loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_browser.Config{
			Name:    "example",
			Verbose: verbose,
		}),
		nil,
	)
}
