package space_exec

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// BuildPluginConfigID selects a native Bldr build of Space-stored source.
const BuildPluginConfigID = "space-exec/build-plugin"

// RegisterBuildPlugin exposes plugin builds to the existing Forge worker owner.
func RegisterBuildPlugin(r *Registry, b bus.Bus) {
	r.Register(BuildPluginConfigID, func(ctx context.Context, le *logrus.Entry, ws world.WorldState, handle forge_target.ExecControllerHandle, inputs forge_target.InputMap, data []byte) (Handler, error) {
		return NewBuildPluginHandlerWithBus(ctx, b, le, ws, handle, inputs, data)
	})
}

// PluginFrontendProtocol identifies one execution's authenticated compiler service.
func PluginFrontendProtocol(id string) protocol.ID {
	return protocol.ID("space/plugin-frontend/" + id)
}
