package assembly_controller

import (
	"github.com/aperturerobotics/bldr/assembly"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
)

// runningAssemblyState implements configset state
type runningAssemblyState struct {
	conf   assembly.Assembly
	err    error
	subAsm []assembly.SubAssemblyState
	cStat  controller_exec.ControllerStatus
}

// GetAssembly returns the current assembly in use.
func (s *runningAssemblyState) GetAssembly() assembly.Assembly {
	return s.conf
}

// GetError returns any error processing the Assembly.
func (s *runningAssemblyState) GetError() error {
	return s.err
}

// GetSubAssemblies returns the list of running SubAssembly.
// May be empty / incomplete until the SubAssemblies are resolved.
func (s *runningAssemblyState) GetSubAssemblies() []assembly.SubAssemblyState {
	return s.subAsm
}

// GetControllerStatus returns the exec controller status.
func (s *runningAssemblyState) GetControllerStatus() controller_exec.ControllerStatus {
	return s.cStat
}

// _ is a type assertion
var _ assembly.State = ((*runningAssemblyState)(nil))
