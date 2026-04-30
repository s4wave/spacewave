package sobject_world_engine

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// executeProcessOpsAsValidator executes processing operations as a validator.
func (c *Controller) executeProcessOpsAsValidator(ctx context.Context, so sobject.SharedObject) error {
	return so.ProcessOperations(
		ctx,
		true,
		func(
			ctx context.Context,
			snap sobject.SharedObjectStateSnapshot,
			currentStateData []byte,
			ops []*sobject.SOOperationInner,
		) (
			rawNextStateData *[]byte,
			opResults []*sobject.SOOperationResult,
			err error,
		) {
			ctx, task := trace.NewTask(ctx, "alpha/validator/process-batch")
			defer task.End()

			le := c.le.
				WithField("ops-stage", "validator").
				WithField("ops-len", len(ops))
			le.Debug("processing ops")

			// Parse the previous state data if it exists
			headState := &InnerState{}
			if err := headState.UnmarshalVT(currentStateData); err != nil {
				return nil, nil, err
			}
			initHeadState := headState.CloneVT()

			// Apply ops
			opResults = make([]*sobject.SOOperationResult, 0, len(ops))
			for i, opInner := range ops {
				// Check the commit result cache before expensive processOp.
				if cached := c.lastCommitResult.Load(); cached != nil &&
					cached.baseRootRef.EqualsRef(headState.GetHeadRef().GetRootRef()) &&
					bytes.Equal(cached.opData, opInner.GetOpData()) {
					headState = cached.resultState
					opPeerID, _ := opInner.ParsePeerID()
					opResults = append(opResults, sobject.BuildSOOperationResult(
						opPeerID.String(), opInner.GetNonce(), true, nil,
					))
					continue
				}

				opPeerID, err := opInner.ParsePeerID()
				if err != nil {
					return nil, nil, err
				}

				nhs, res, err := c.processOp(
					ctx,
					le,
					so,
					opInner.GetOpData(),
					opInner.GetLocalId(),
					opPeerID,
					opInner.GetNonce(),
					i,
					headState,
				)
				if err != nil {
					return nil, nil, err
				}
				if res != nil {
					opResults = append(opResults, res)
				}
				if nhs != nil {
					headState = nhs
				}
			}

			// If no state changes occurred, return no-op
			if headState.EqualVT(initHeadState) {
				le.Debug("no state changes")
				return nil, opResults, nil
			}

			// Marshal the next state
			nextStateData, err := headState.MarshalVT()
			if err != nil {
				return nil, nil, err
			}

			le.Debug("processed ops")
			return &nextStateData, opResults, nil
		},
	)
}
