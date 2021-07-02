package pass_controller

import "errors"

var (
	// ErrNotExecController is returned if a exec.controller does not implement
	// target.ExecController.
	ErrNotExecController = errors.New("controller does not implement ExecController")
)
