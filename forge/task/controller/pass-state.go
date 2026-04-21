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

/*
// checkChanged checks if the two states are different.
func (s *passState) checkChanged(ot *passState) bool {
	switch {
	case ot.objKey != s.objKey:
	case (s.pass == nil || ot.pass == nil) && (s.pass != ot.pass):
	case s.pass.GetPassState() != ot.pass.GetPassState():
	case !s.pass.GetResult().Equals(ot.pass.GetResult()):
	default:
		return false
	}
	return true
}
*/
