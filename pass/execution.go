package forge_pass

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// CreateExecutionWithPass creates a pending Execution object for a Pass.
//
// Writes the Target to a block linked to by the Execution.
// execPeerID is the peer id to assign to the execution.
func CreateExecutionWithPass(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	execObjKey string,
	passObjKey string,
	passObjBcs *block.Cursor,
	passObj *Pass,
	execPeerID peer.ID,
) (*bucket.ObjectRef, error) {
	if len(execPeerID) == 0 {
		return nil, peer.ErrPeerIDEmpty
	}
	if passObjKey == "" || execObjKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	tgt, _, err := passObj.FollowTargetRef(passObjBcs)
	if err != nil {
		return nil, err
	}
	if err := tgt.Validate(); err != nil {
		return nil, err
	}

	valueSet := passObj.GetValueSet().Clone()
	valueSet.Outputs = nil

	return forge_execution.CreateExecutionWithTarget(
		ctx,
		ws,
		sender,
		execObjKey,
		execPeerID,
		valueSet,
		tgt,
		passObj.GetTimestamp().Clone(),
	)
}
