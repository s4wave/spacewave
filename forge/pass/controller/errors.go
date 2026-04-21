package pass_controller

import "errors"

// ErrNotExecController is returned if a exec.controller does not implement
// target.ExecController.
var ErrNotExecController = errors.New("controller does not implement ExecController")
