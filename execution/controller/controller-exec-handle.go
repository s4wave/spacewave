package execution_controller

import forge_target "github.com/aperturerobotics/forge/target"

// execControllerHandle implements ExecControllerHandle from target.
type execControllerHandle struct {
	c *Controller
}

// newExecControllerHandle constructs an ExecControllerHandle.
func newExecControllerHandle(c *Controller) *execControllerHandle {
	return &execControllerHandle{c: c}
}

// _ is a type assertion
var _ forge_target.ExecControllerHandle = ((*execControllerHandle)(nil))
