package space_exec_noop

import (
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// ConfigID is the config ID for the noop handler.
const ConfigID = "space-exec/noop"

// NewTarget returns a Forge target that runs through the noop bridge.
func NewTarget() *forge_target.Target {
	return &forge_target.Target{
		Exec: &forge_target.Exec{
			Controller: &configset_proto.ControllerConfig{
				Id:  ConfigID,
				Rev: 1,
			},
		},
	}
}
