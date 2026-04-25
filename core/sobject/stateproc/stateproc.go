package sobject_stateproc

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// ApplyOpFunc applies a single operation to the current state.
// stateData is the current marshaled state (nil/empty for uninitialized).
// opData is the operation payload from the SOOperationInner.
// sender is the peer ID that submitted the operation.
// Returns the next marshaled state data.
// Return an error to reject the operation.
type ApplyOpFunc func(
	ctx context.Context,
	stateData []byte,
	opData []byte,
	sender peer.ID,
) ([]byte, error)

// BuildProcessOpsFunc wraps an ApplyOpFunc into a sobject.ProcessOpsFunc.
// The resulting function processes operations in batch, applying each one
// sequentially and collecting results.
func BuildProcessOpsFunc(applyOp ApplyOpFunc) sobject.ProcessOpsFunc {
	return func(
		ctx context.Context,
		snap sobject.SharedObjectStateSnapshot,
		currentStateData []byte,
		ops []*sobject.SOOperationInner,
	) (*[]byte, []*sobject.SOOperationResult, error) {
		stateData := currentStateData
		changed := false
		opResults := make([]*sobject.SOOperationResult, 0, len(ops))

		for _, opInner := range ops {
			opPeerID, err := opInner.ParsePeerID()
			if err != nil {
				return nil, nil, err
			}

			nextState, applyErr := applyOp(ctx, stateData, opInner.GetOpData(), opPeerID)
			if applyErr != nil {
				opResults = append(opResults, sobject.BuildSOOperationResult(
					opPeerID.String(), opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{
						ErrorMsg: applyErr.Error(),
					},
				))
				continue
			}

			stateData = nextState
			changed = true
			opResults = append(opResults, sobject.BuildSOOperationResult(
				opPeerID.String(), opInner.GetNonce(), true, nil,
			))
		}

		if !changed {
			return nil, opResults, nil
		}
		return &stateData, opResults, nil
	}
}
