//go:build !js

package spacewave_compose

import (
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	spacewave_cli "github.com/s4wave/spacewave/cmd/spacewave/cli"
	launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	resource_root_controller "github.com/s4wave/spacewave/core/resource/root/controller"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
)

// composeNative adds the native host controllers and CLI commands.
//
// The process owns one yield broker and one listener status broker, shared by
// the resource listener, the root resource controller, the status projector and
// the CLI commands.
func composeNative(composition *compose.Composition) {
	yield := yield_policy.NewBroker()
	status := resource_listener.NewStatusBroker()

	// Share the brokers with the controllers that serve and project the
	// resource listener.
	composition.Factories = append(composition.Factories, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{
			resource_listener.NewFactory(
				b,
				resource_listener.WithYieldBroker(yield),
				resource_listener.WithStatusBroker(status),
			),
			resource_root_controller.NewFactory(
				b,
				resource_root_controller.WithYieldBroker(yield),
				resource_root_controller.WithListenerStatusBroker(status),
			),
			statusprojector.NewFactory(b, statusprojector.WithListenerStatusBroker(status)),

			// Installed hosts retain the application operation factory for
			// local worlds.
			space_world_optypes.NewFactory(b),

			// Replacement and relaunch target this process's executable and
			// lifetime.
			launcher.NewFactory(b),
		}
	})

	// A command process never displaces the foreground serve process: its
	// first bus access begins a handoff, and serve binds the socket itself
	// after installing its own handoff guard.
	composition.Commands = append(composition.Commands, func(getBus func() cli_entrypoint.CliBus) []*cli.Command {
		handedOff := false
		protectedGetBus := func() cli_entrypoint.CliBus {
			if !handedOff {
				yield.BeginHandoff("spacewave CLI", "")
				handedOff = true
			}
			return getBus()
		}
		return spacewave_cli.NewCliCommands(protectedGetBus, yield)
	})
}
