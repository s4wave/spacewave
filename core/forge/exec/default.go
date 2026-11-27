package space_exec

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	space_exec_noop "github.com/s4wave/spacewave/core/forge/exec/noop"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// NewDefaultRegistry creates a registry with all built-in space exec handlers.
func NewDefaultRegistry() *Registry {
	return NewDefaultRegistryWithBus(nil)
}

// NewDefaultRegistryWithBus creates a registry with all built-in space exec
// handlers, including handlers that need the controller bus.
func NewDefaultRegistryWithBus(b bus.Bus) *Registry {
	// Register the built-in handlers that execute directly against Space data.
	r := NewRegistry()
	r.Register(NoopConfigID, NewNoopHandler)
	r.Register(KvtxConfigID, NewKvtxHandler)
	r.Register(GitCloneConfigID, NewGitCloneHandler)
	r.Register(UnixfsReadConfigID, NewUnixfsReadHandler)
	r.Register(FileHashConfigID, NewFileHashHandler)
	r.Register(ExportZipConfigID, NewExportZipHandler)

	// Register the VM and plugin handlers with their controller bus.
	r.Register(V86ConfigID, NewV86Handler(b))
	r.Register(PluginExecConfigID, NewPluginExecHandler(b))
	r.Register(BuildPluginConfigID, NewBuildPluginHandler(b))
	return r
}

// BridgeFactories returns bus-compatible controller factories for all handlers
// in the registry. Each handler gets a BridgeFactory that responds to
// LoadConfigConstructorByID and LoadFactoryByConfig on the bus, making all
// space-exec handlers discoverable through the standard forge execution
// controller dispatch. Other plugins can contribute additional handlers by
// registering their own controller factories on the bus.
func BridgeFactories(r *Registry) []controller.Factory {
	ids := r.ConfigIDs()
	factories := make([]controller.Factory, 0, len(ids))
	for _, id := range ids {
		factories = append(factories, NewBridgeFactory(id, r))
	}
	return factories
}

// NewNoopTarget returns a Forge target that runs through the noop bridge.
func NewNoopTarget() *forge_target.Target {
	return space_exec_noop.NewTarget()
}
