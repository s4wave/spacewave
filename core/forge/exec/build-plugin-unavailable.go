//go:build js || tinygo

package space_exec

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// NewBuildPluginHandlerWithBus requires a native device with the Bldr toolchain.
func NewBuildPluginHandlerWithBus(context.Context, bus.Bus, *logrus.Entry, world.WorldState, forge_target.ExecControllerHandle, forge_target.InputMap, []byte) (Handler, error) {
	return nil, errors.New("plugin builds require a registered native build device")
}
