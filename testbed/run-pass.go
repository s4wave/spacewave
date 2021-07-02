package testbed

import (
	"errors"

	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_target "github.com/aperturerobotics/forge/target"
)

// RunPassWithTarget runs a target using the Pass and Execution controllers.
func (tb *Testbed) RunPassWithTarget(
	tgt *forge_target.Target,
	valueSet *forge_target.ValueSet,
) (*forge_pass.Pass, error) {
	ctx, le, ws := tb.Context, tb.Logger, tb.WorldState

	targetObjectKey := "targets/1"
	_, tgtRef, err := forge_target.CreateTarget(ctx, tb.WorldState, targetObjectKey, tgt)
	if err != nil {
		return nil, err
	}

	_ = tgtRef
	_ = le
	_ = ws
	return nil, errors.New("TODO run pass controller")
}
