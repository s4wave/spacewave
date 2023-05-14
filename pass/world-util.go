package forge_pass

import (
	"context"

	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/cayleygraph/cayley"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CheckPassType checks the type graph quad for a Pass.
func CheckPassType(ctx context.Context, ws world.WorldState, objKey string) error {
	return world_types.CheckObjectType(ctx, ws, objKey, PassTypeID)
}

// LookupPass looks up a Pass in the world.
func LookupPass(ctx context.Context, ws world.WorldState, objKey string) (*Pass, *forge_target.Target, error) {
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var pass *Pass
	var tgt *forge_target.Target
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		pass, err = UnmarshalPass(ctx, bcs)
		if err == nil && !pass.GetTargetRef().GetEmpty() {
			tgt, _, err = pass.FollowTargetRef(ctx, bcs)
		}
		return err
	})
	return pass, tgt, err
}

// WaitPassComplete waits until the Pass is in the COMPLETE state.
func WaitPassComplete(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	passObjectKey string,
) (*Pass, error) {
	// wait for Pass to complete
	var finalState *Pass
	var lastState State
	loop := world_control.NewWatchLoop(
		le,
		passObjectKey,
		world_control.NewWaitForStateHandler(
			func(ctx context.Context, ws world.WorldState, obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				pass, err := UnmarshalPass(ctx, rootCs)
				if err != nil {
					return false, err
				}
				nextState := pass.GetPassState()
				if nextState != lastState {
					lastState = nextState
					le.Debugf("pass is in state: %s", nextState.String())
					if ferr := pass.GetResult().GetFailError(); ferr != "" {
						le.WithError(errors.New(ferr)).Warn("pass failed")
					}
				}
				complete := pass.IsComplete()
				if complete {
					finalState = pass
				}
				return !complete, nil
			},
		),
	)
	if err := loop.Execute(ctx, ws); err != nil {
		return nil, err
	}
	return finalState, nil
}

// ListPassExecutions lists all Execution object keys that are linked to by the Pass.
func ListPassExecutions(ctx context.Context, w world.WorldState, passKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		passKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredPassToExecution), nil
		},
	)
}

// CollectPassExecutions collects all Executions linked to by the Pass.
// If any of the linked states are invalid, returns an error.
func CollectPassExecutions(
	ctx context.Context,
	ws world.WorldState,
	passObjectKeys ...string,
) ([]*forge_execution.Execution, []string, error) {
	kpObjectKeys, err := ListPassExecutions(ctx, ws, passObjectKeys...)
	if err != nil {
		return nil, nil, err
	}

	states := make([]*forge_execution.Execution, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], err = forge_execution.LookupExecution(ctx, ws, objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return states, kpObjectKeys, nil
}
