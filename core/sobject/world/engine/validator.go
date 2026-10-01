package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// executeProcessOpsWhenValidator waits until this participant can validate and
// then processes queued operations. Writer and reader copies receive validated
// roots from their Space peers; they must not attempt to finalize those roots.
func (c *Controller) executeProcessOpsWhenValidator(
	ctx context.Context,
	so sobject.SharedObject,
	state ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) error {
	for {
		snapshot := state.GetValue()
		if snapshot == nil {
			if _, err := state.WaitValue(ctx, nil); err != nil {
				return err
			}
			continue
		}
		// A departed participant waits for readmission like a non-validator.
		participant, err := snapshot.GetParticipantConfig(ctx)
		if err != nil && !isReadAccessLoss(err) {
			return err
		}
		if err == nil && sobject.IsValidatorOrOwner(participant.GetRole()) {
			return c.executeProcessOpsAsValidator(ctx, so)
		}
		if _, err := state.WaitValueChange(ctx, snapshot, nil); err != nil {
			return err
		}
	}
}

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
			// Trace and log the batch.
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
				// Replay the operation as its signer.
				opPeerID, err := opInner.ParsePeerID()
				if err != nil {
					return nil, nil, err
				}
				nhs, res, err := c.replayOp(ctx, le, snap, so, opInner, opPeerID, i, headState)
				if err != nil {
					return nil, nil, err
				}

				// Retain the World and any changed retained roots before they
				// can become accepted. A block missing locally and from storage
				// rejects only its operation.
				if nhs != nil {
					err := c.retainPublicationWorld(ctx, so, nhs.GetHeadRef())
					if err == nil && !slices.EqualFunc(nhs.GetRetainedRoots(), headState.GetRetainedRoots(), (*RetainedRoot).EqualVT) {
						err = c.retainRoots(ctx, so, nhs.GetRetainedRoots())
					}
					if err != nil && !errors.Is(err, block.ErrNotFound) {
						return nil, nil, err
					}
					if err != nil {
						le.WithError(err).Warn("rejecting op: world block is missing")
						nhs = nil
						res = sobject.BuildSOOperationResult(
							opPeerID.String(),
							opInner.GetNonce(),
							false,
							&sobject.SOOperationRejectionErrorDetails{
								ErrorMsg:     "world block is missing: " + err.Error(),
								MissingBlock: true,
							},
						)
					}
				}
				if res != nil {
					opResults = append(opResults, res)
				}
				if nhs == nil {
					continue
				}

				// Only the current replay candidate needs its own root. Its
				// immutable history and the accepted head retain their data.
				headState = nhs
				if err := block.SetRetainedRoot(ctx, so.GetBlockStore(), "validator-world", headState.GetHeadRef().GetRootRef()); err != nil {
					return nil, nil, err
				}
			}

			// If no state changes occurred, return no-op
			if headState.EqualVT(initHeadState) {
				le.Debug("no state changes")
				return nil, opResults, nil
			}

			// Propose the next state.
			nextStateData, err := headState.MarshalVT()
			if err != nil {
				return nil, nil, err
			}
			le.Debug("processed ops")
			return &nextStateData, opResults, nil
		},
	)
}

// replayOp computes the next state and result of one queued operation. It
// adopts the foreground commit result instead of replaying the same operation
// on the same base.
func (c *Controller) replayOp(
	ctx context.Context,
	le *logrus.Entry,
	snap sobject.SharedObjectStateSnapshot,
	so sobject.SharedObject,
	opInner *sobject.SOOperationInner,
	opPeerID peer.ID,
	opIdx int,
	headState *InnerState,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Adopt the foreground result of the same operation on the same base and
	// storage generation.
	cached := c.lastCommitResult.Load()
	if cached != nil &&
		cached.baseRootRef.EqualsRef(headState.GetHeadRef().GetRootRef()) &&
		cached.storageGeneration == headState.GetStorageGeneration() &&
		bytes.Equal(cached.opData, opInner.GetOpData()) {
		next := headState.CloneVT()
		next.HeadRef = cached.resultRef.CloneVT()
		return next, sobject.BuildSOOperationResult(opPeerID.String(), opInner.GetNonce(), true, nil), nil
	}

	// Authenticated World operations attribute the signer's accepted person.
	person, err := operationPerson(ctx, snap, opPeerID)
	if err != nil {
		return nil, nil, err
	}
	return c.processOp(
		world.WithOperationPerson(ctx, person),
		le,
		so,
		opInner.GetOpData(),
		opInner.GetLocalId(),
		opPeerID,
		opInner.GetNonce(),
		opIdx,
		headState,
	)
}
