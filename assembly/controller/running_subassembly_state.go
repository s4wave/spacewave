package assembly_controller

import (
	"github.com/aperturerobotics/bldr/assembly"
)

// runningSubAssemblyState implements configset state
type runningSubAssemblyState struct {
	conf assembly.SubAssembly
	err  error
	asms []assembly.State
}

// GetSubAssembly returns the SubAssembly described by this State.
// May be empty until the SubAssembly is resolved
func (s *runningSubAssemblyState) GetSubAssembly() assembly.SubAssembly {
	return s.conf
}

// GetError returns any error processing the SubAssembly.
func (s *runningSubAssemblyState) GetError() error {
	return s.err
}

// GetAssemblies returns the list of running Assembly on the bus.
// May be empty / incomplete until the Assemblies are resolved.
// (This is a pass-through of the ApplyAssembly states).
func (s *runningSubAssemblyState) GetAssemblies() []assembly.State {
	return s.asms
}

// _ is a type assertion
var _ assembly.SubAssemblyState = ((*runningSubAssemblyState)(nil))
