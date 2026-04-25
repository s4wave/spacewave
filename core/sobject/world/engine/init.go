package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// loadOrInitHeadFromSharedObject loads the head state from the shared object.
//
// if the shared object is empty:
//
// - if we are a validator: generate the initial state and update the SharedObject.
// - if we are a writer or reader: wait for an initial state to exist.
func (c *Controller) loadOrInitHeadFromSharedObject(
	ctx context.Context,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) (*InnerState, error) {
	snap, err := soStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Wait until the head ref is set.
	var appliedInitOp bool
	for {
		// returns ErrNotParticipant if the local peer is not a participant.
		localParticipant, err := snap.GetParticipantConfig(ctx)
		if err != nil {
			return nil, err
		}

		currState, err := snap.GetRootInner(ctx)
		if err != nil {
			return nil, err
		}

		innerStateData := currState.GetStateData()
		innerState := &InnerState{}
		if err := innerState.UnmarshalVT(innerStateData); err != nil {
			return nil, err
		}

		// not empty => exit loop
		headRef := innerState.GetHeadRef()
		if headRef != nil {
			headRef.BucketId = ""
		}
		if !headRef.GetEmpty() {
			if err := headRef.Validate(); err != nil {
				return nil, err
			}
			return innerState, nil
		}

		// empty => continue to wait
		opQueue, localOpQueue, err := snap.GetOpQueue(ctx)
		if err != nil {
			return nil, err
		}
		le := c.le.WithFields(logrus.Fields{
			"so-seqno": currState.GetSeqno(),
		})

		if !appliedInitOp &&
			len(opQueue) == 0 &&
			len(localOpQueue) == 0 &&
			sobject.IsValidatorOrOwner(localParticipant.GetRole()) {
			le.Debug("submitting operation to initialize shared object state")

			initWorldOp := c.conf.GetInitWorldOp().CloneVT()
			if initWorldOp == nil {
				initWorldOp = &InitWorldOp{}
			}

			opData, err := (&SOWorldOp{
				Body: &SOWorldOp_InitWorld{
					InitWorld: initWorldOp,
				},
			}).MarshalVT()
			if err != nil {
				return nil, err
			}

			opID, err := so.QueueOperation(ctx, opData)
			if err != nil {
				return nil, err
			}

			le.Debugf("queued op to init world state: %s", opID)

			_, _, err = so.WaitOperation(ctx, opID)
			if err != nil {
				if ctx.Err() != nil {
					return nil, context.Canceled
				}
				le.WithError(err).Warn("initing world state failed")
				return nil, err
			}

			appliedInitOp = true
		}

		// wait for the state to change
		snap, err = soStateCtr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return nil, err
		}
	}
}
