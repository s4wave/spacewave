package sobject_world_engine

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/sirupsen/logrus"
)

// executeWatchSOState watches and processes shared object state changes.
func (c *Controller) executeWatchSOState(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	soEngine *soEngine,
) error {
	var snap sobject.SharedObjectStateSnapshot
	var err error
	for {
		// Wait for the state container value to change.
		_, err = soStateCtr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}

		// Lock the writeMtx, so that we wait until any write txn is done processing first.
		lockCtx, lockTask := trace.NewTask(ctx, "alpha/watch-state/lock-write-mtx")
		unlockWriteMtx, err := c.writeMtx.Lock(lockCtx)
		lockTask.End()
		if err != nil {
			return err
		}

		// Separate lock acquisition from hold time so traces show contention vs work.
		holdCtx, holdTask := trace.NewTask(ctx, "alpha/watch-state/hold-write-mtx")

		// Get the latest snap in case it changed in the meantime.
		snap = soStateCtr.GetValue()

		// Watch the state once (sync any changes to soEngine and update local state).
		err = c.executeWatchSOStateOnce(holdCtx, le, so, snap, soEngine)

		// Be sure to unlock the writeMtx right away.
		holdTask.End()
		unlockWriteMtx()

		// Return the error, if any.
		if err != nil {
			return err
		}
	}
}

// executeWatchSOStateOnce processes a single shared object state change.
func (c *Controller) executeWatchSOStateOnce(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	snap sobject.SharedObjectStateSnapshot,
	soEngine *soEngine,
) error {
	ctx, task := trace.NewTask(ctx, "alpha/watch-state/process-snapshot")
	defer task.End()

	// Get current state
	var currState *sobject.SORootInner
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/watch-state/get-root-inner")
		var err error
		currState, err = snap.GetRootInner(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	// Get operation queue
	var opQueue []*sobject.SOOperation
	var localOpQueue []*sobject.QueuedSOOperation
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/watch-state/get-op-queue")
		var err error
		opQueue, localOpQueue, err = snap.GetOpQueue(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	le = le.WithFields(logrus.Fields{
		"ops-stage": "watch-state",
		"so-seqno":  currState.GetSeqno(),
	})

	le.WithFields(logrus.Fields{
		"so-op-queue-len":       len(opQueue),
		"so-local-op-queue-len": len(localOpQueue),
	}).Debug("processing shared object state")

	// Parse inner state
	innerStateData := currState.GetStateData()
	innerState := &InnerState{}
	if err := innerState.UnmarshalVT(innerStateData); err != nil {
		return err
	}

	if innerState.GetHeadRef() == nil {
		innerState.HeadRef = &bucket.ObjectRef{}
	} else if err := innerState.GetHeadRef().Validate(); err != nil {
		return err
	}

	// Apply any pending operations in the queue to the state
	if len(opQueue) != 0 || len(localOpQueue) != 0 {
		le.WithFields(logrus.Fields{
			"op-queue-len":       len(opQueue),
			"local-op-queue-len": len(localOpQueue),
		}).Debug("applying pending ops to local engine state")

		var xfrm *block_transform.Transformer
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/watch-state/get-transformer")
			var err error
			xfrm, err = snap.GetTransformer(taskCtx)
			task.End()
			if err != nil {
				return err
			}
		}

		// Process main op queue first
		if len(opQueue) != 0 {
			taskCtx, task := trace.NewTask(ctx, "alpha/watch-state/process-authoritative-queue")
			for i, op := range opQueue {
				opInner, err := op.UnmarshalInner()
				if err != nil {
					task.End()
					return err
				}

				// transform the op data
				decOpData, err := xfrm.DecodeBlock(opInner.GetOpData())
				if err != nil {
					task.End()
					return err
				}

				// Check the commit result cache before expensive processOp.
				if cached := c.lastCommitResult.Load(); cached != nil &&
					cached.baseRootRef.EqualsRef(innerState.GetHeadRef().GetRootRef()) &&
					bytes.Equal(cached.opData, decOpData) {
					innerState = cached.resultState
					continue
				}

				opPeerID, err := opInner.ParsePeerID()
				if err != nil {
					task.End()
					return err
				}

				nhs, _, err := c.processOp(
					taskCtx,
					le,
					so,
					decOpData,
					opInner.GetLocalId(),
					opPeerID,
					opInner.GetNonce(),
					i,
					innerState,
				)
				if err != nil {
					task.End()
					return err
				}
				if nhs != nil {
					innerState = nhs
				}
			}
			task.End()
		}

		// Then process local op queue
		if len(localOpQueue) != 0 {
			taskCtx, task := trace.NewTask(ctx, "alpha/watch-state/process-local-queue")
			startIdx := len(opQueue)
			for i, queuedOp := range localOpQueue {
				// Check the commit result cache before expensive processOp.
				if cached := c.lastCommitResult.Load(); cached != nil &&
					cached.baseRootRef.EqualsRef(innerState.GetHeadRef().GetRootRef()) &&
					bytes.Equal(cached.opData, queuedOp.GetOpData()) {
					innerState = cached.resultState
					continue
				}

				nhs, _, err := c.processOp(
					taskCtx,
					le,
					so,
					queuedOp.GetOpData(),
					queuedOp.GetLocalId(),
					so.GetPeerID(), // local peer id
					0,              // no nonce for queued ops
					startIdx+i,
					innerState,
				)
				if err != nil {
					task.End()
					return err
				}
				if nhs != nil {
					innerState = nhs
				}
			}
			task.End()
		}
	}

	// Apply the new state to soEngine
	taskCtx, task2 := trace.NewTask(ctx, "alpha/watch-state/update-engine-state")
	err := soEngine.updateEngineState(taskCtx, innerState.GetHeadRef())
	task2.End()
	if err == nil {
		c.notifyGCSweepMaintenance()
	}
	return err
}
