package task_controller

import forge_pass "github.com/s4wave/spacewave/forge/pass"

// passState is a snapshot of the pass state.
type passState struct {
	// objKey is the object key
	objKey string
	// pass is the most recent pass object
	pass *forge_pass.Pass
}

// newPassState constructs a new pass state.
func newPassState(objKey string, pass *forge_pass.Pass) *passState {
	return &passState{
		objKey: objKey,
		pass:   pass,
	}
}
