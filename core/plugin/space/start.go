package plugin_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
)

// StartControllerWithConfig starts the space plugin controller with a config.
// Waits for the controller to start running.
// Returns a Release function to close the controller when done.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	rel func(),
) (*Controller, directive.Instance, directive.Reference, error) {
	return loader.WaitExecControllerRunningTyped[*Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		rel,
	)
}
