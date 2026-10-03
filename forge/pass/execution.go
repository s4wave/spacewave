package forge_pass

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
	"github.com/s4wave/spacewave/net/peer"
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
	// Validate the peer ID and object keys.
	if len(execPeerID) == 0 {
		return nil, peer.ErrEmptyPeerID
	}
	if passObjKey == "" || execObjKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Follow and validate the pass target reference.
	tgt, _, err := passObj.FollowTargetRef(ctx, passObjBcs)
	if err != nil {
		return nil, err
	}
	if err := tgt.Validate(); err != nil {
		return nil, err
	}

	// Create the Execution with a copy of the pass value set.
	valueSet := passObj.GetValueSet().Clone()
	valueSet.Outputs = nil

	// Create the pending Execution before binding its Job checkout grant.
	ref, err := forge_execution.CreateExecutionWithTarget(
		ctx,
		ws,
		sender,
		execObjKey,
		execPeerID,
		valueSet,
		tgt,
		passObj.GetPlacement(),
		passObj.GetTimestamp().CloneVT(),
	)
	if err != nil {
		return nil, err
	}

	// Follow Pass containment to its Task and retained Job without scanning history.
	taskKey, err := world_parent.GetObjectParent(ctx, ws, passObjKey)
	if err != nil {
		return nil, err
	}
	if taskKey != "" {
		jobKey, err := world_parent.GetObjectParent(ctx, ws, taskKey)
		if err != nil {
			return nil, err
		}
		if jobKey != "" {
			if err := forge_allocation.BindJobAllocations(ctx, ws, jobKey, execObjKey); err != nil {
				return nil, err
			}
		}
	}
	return ref, nil
}
