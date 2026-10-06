package sobject_world_engine

import (
	"context"
	"errors"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// waitWorldInit replays the shared object state until its World is
// initialized and returns that World. An owner whose replay finds no World adds
// the operation that initializes it; when several owners do, replay keeps the
// first and rejects the rest. A replay that stops at a missing block returns
// the initialized World before the stalled operation, so the World is served
// and the watcher resumes the replay at that operation.
func (c *Controller) waitWorldInit(
	ctx context.Context,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	replay *replayer,
) (*InnerState, error) {
	snap, err := soStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}

	var queued bool
	for {
		// Replay the state. ErrNotParticipant ends the wait.
		participant, err := snap.GetParticipantConfig(ctx)
		if err != nil {
			return nil, err
		}
		state, _, err := replay.sync(ctx, snap, nil)
		stalled := state != nil && errors.Is(err, block.ErrNotFound)
		if stalled {
			c.le.WithError(err).Warn("replay waits for a block that is not available")
		} else if err != nil {
			return nil, err
		}
		if head := state.GetHeadRef(); !head.GetEmpty() {
			if err := head.Validate(); err != nil {
				return nil, err
			}
			return state, nil
		}

		// Initialize the World once as an owner whose replay is complete.
		if !queued && !stalled && sobject.IsOwner(participant.GetRole()) {
			opData, err := c.buildInitWorldOp()
			if err != nil {
				return nil, err
			}
			localID, err := so.QueueOperation(ctx, opData)
			if err != nil {
				return nil, err
			}
			c.le.Debugf("queued op to init world state: %s", localID)
			queued = true
		}

		// Wait for the state to change.
		snap, err = soStateCtr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return nil, err
		}
	}
}

// buildInitWorldOp encodes the operation that initializes the World.
func (c *Controller) buildInitWorldOp() ([]byte, error) {
	initOp, err := NewInitWorldOp(c.conf.GetInitWorldOp())
	if err != nil {
		return nil, err
	}
	return (&SOWorldOp{Body: &SOWorldOp_InitWorld{InitWorld: initOp}}).MarshalVT()
}
