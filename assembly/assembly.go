package assembly

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
)

// Assembly is a set of configurations to load for an application.
type Assembly interface {
	// ResolveControllerExec resolves the controller exec configuration for the Assembly.
	// return nil if no controller exec configured.
	ResolveControllerExec(ctx context.Context, b bus.Bus) (*controller_exec.ExecControllerRequest, error)
	// ResolveSubAssemblies resolves the list of sub assembly bus to run.
	// Can be configured to optionally inherit parent plugins and resolvers.
	ResolveSubAssemblies(ctx context.Context, b bus.Bus) ([]SubAssembly, error)
}

// SubAssembly configures a separate Bus to be run as a child.
// Can be configured to optionally inherit parent plugins and resolvers.
type SubAssembly interface {
	// GetId returns the subassembly ID, used for logging and identification.
	// Can be empty.
	GetId() string
	// ResolveAssemblies resolves the list of assembly to run on the SubAssembly bus.
	ResolveAssemblies(ctx context.Context, b bus.Bus) ([]Assembly, error)
	// ResolveDirectiveBridges resolves the list of directive bridges to apply.
	ResolveDirectiveBridges(ctx context.Context, b bus.Bus) ([]DirectiveBridge, error)
}

// Controller is a assembly controller.
type Controller interface {
	// Controller indicates this is a controllerbus controller.
	controller.Controller

	// PushAssembly pushes an assembly to run.
	// returns nil, ErrEmptyAssembly if the conf was nil.
	PushAssembly(
		ctx context.Context,
		conf Assembly,
	) (Reference, error)
}

// Reference is a reference to monitor the state of a pushed Assembly.
// Will automatically be released if the directive is removed.
type Reference interface {
	// GetState returns the current state object.
	GetState() State
	// AddStateCb adds a callback that is called when the state changes.
	// Should not block.
	// Will be called with the initial state.
	AddStateCb(func(State))
	// Release releases the reference.
	Release()
}

// State contains Assembly state information.
type State interface {
	// GetAssembly returns the assembly described by this State.
	// May be empty until the Assembly is resolved
	GetAssembly() Assembly
	// GetError returns any error processing the Assembly.
	// Note: unless DisablePartialSuccess, SubAssemblies might individually error.
	GetError() error
	// GetSubAssemblies returns the list of running SubAssembly.
	// May be empty / incomplete until the SubAssemblies are resolved.
	GetSubAssemblies() []SubAssemblyState
	// GetControllerStatus returns the exec controller status.
	GetControllerStatus() controller_exec.ControllerStatus
	// TODO controller states, configset states
}

// SubAssemblyState contains SubAssembly state information.
type SubAssemblyState interface {
	// GetSubAssembly returns the SubAssembly described by this State.
	// May be empty until the SubAssembly is resolved
	GetSubAssembly() SubAssembly
	// GetError returns any error processing the SubAssembly.
	GetError() error
	// GetAssemblies returns the list of running Assembly on the bus.
	// May be empty / incomplete until the Assemblies are resolved.
	// (This is a pass-through of the ApplyAssembly states).
	GetAssemblies() []State
}
