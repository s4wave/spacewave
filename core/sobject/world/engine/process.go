package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// processOp processes a single operation and returns the next state and
// operation result. An operation with no peerID is local, and its rejection
// is returned as an error.
func (c *Controller) processOp(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	opData []byte,
	localID string,
	peerID peer.ID,
	nonce uint64,
	opIdx int,
	headState *InnerState,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Trace and decode the operation.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/process-op")
	defer task.End()
	op := &SOWorldOp{}
	if err := op.UnmarshalVT(opData); err != nil {
		return rejectOp(le, peerID, nonce, "invalid operation data: "+err.Error())
	}

	// Log under the operation's identity.
	ole := le.WithFields(logrus.Fields{
		"op-idx":      opIdx,
		"op-local-id": localID,
		"op-nonce":    nonce,
		"op-peer-id":  peerID.String(),
	})
	ole.Debug("processing op")

	// Apply the operation by type.
	switch body := op.GetBody().(type) {
	case *SOWorldOp_InitWorld:
		return c.processInitWorldOp(
			ctx,
			ole,
			so,
			body.InitWorld,
			headState,
			peerID,
			nonce,
		)
	case *SOWorldOp_ApplyTxOp:
		if headState.GetHeadRef().GetEmpty() {
			return rejectOp(ole, peerID, nonce, "world is not initialized")
		}

		// Build world state with engine once for all operations
		var ws *blkEngine
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-op/build-block-engine")
			var err error
			ws, err = c.buildBlkEngine(taskCtx, le, so, headState.GetHeadRef(), headState.GetHeadRef().GetTransformConf())
			task.End()
			if err != nil {
				return nil, nil, err
			}
		}
		defer ws.Release()

		// Process ApplyTxOp using the shared world state
		nhs, res, err := c.processApplyTxOpWithEngine(
			ctx,
			ole,
			body.ApplyTxOp,
			headState,
			peerID,
			nonce,
			ws,
		)
		if err != nil {
			return nil, nil, err
		}
		if res != nil {
			ole.Debugf("applied world txn op: %v", body.ApplyTxOp.GetTx().GetTxType().String())
		}
		return nhs, res, nil
	case *SOWorldOp_SetRetainedRoot:
		return processSetRetainedRootOp(ole, body.SetRetainedRoot, headState, peerID, nonce)
	default:
		ole.Warn("rejecting op: unknown op type")
		return nil, opRejection(peerID, nonce, "unknown operation type"), nil
	}
}

// processInitWorldOp processes an InitWorld operation.
func (c *Controller) processInitWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	initOp *InitWorldOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Refuse to initialize a World twice.
	if !headState.GetHeadRef().GetEmpty() {
		le.Warn("rejecting world init op: world is already initialized")
		return nil, opRejection(peerID, nonce, "world is already initialized"), nil
	}

	// Build the initial World and persist its root. The transform is part of
	// the operation, so a missing or unauthenticated one is a rejection every
	// member agrees on.
	finalState, err := BuildInitialInnerState(initOp)
	if err != nil {
		le.WithError(err).Warn("rejecting world init op")
		return nil, opRejection(peerID, nonce, "invalid world init: "+err.Error()), nil
	}
	if err := c.writeInitialWorldRoot(ctx, le, so, initOp, finalState.GetHeadRef()); err != nil {
		return nil, nil, err
	}
	return finalState, sobject.BuildSOOperationResult(peerID.String(), nonce, true, nil), nil
}

// rejectOp rejects the operation nonce submitted by peerID with msg, or
// returns msg as an error for a local operation, which has no submitter.
func rejectOp(le *logrus.Entry, peerID peer.ID, nonce uint64, msg string) (*InnerState, *sobject.SOOperationResult, error) {
	if peerID == "" {
		return nil, nil, errors.New(msg)
	}
	le.Warn("rejecting op: " + msg)
	return nil, opRejection(peerID, nonce, msg), nil
}

// opRejection rejects the operation nonce submitted by peerID with msg.
func opRejection(peerID peer.ID, nonce uint64, msg string) *sobject.SOOperationResult {
	return sobject.BuildSOOperationResult(
		peerID.String(),
		nonce,
		false,
		&sobject.SOOperationRejectionErrorDetails{ErrorMsg: msg},
	)
}

// writeInitialWorldRoot persists an initial World when changelog initialization is disabled.
func (c *Controller) writeInitialWorldRoot(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	initOp *InitWorldOp,
	headRef *bucket.ObjectRef,
) error {
	if !initOp.GetLastChangeDisable() {
		return nil
	}
	if so == nil {
		return errors.New("shared object is required to initialize disabled changelog world")
	}
	if headRef.GetTransformConf().GetEmpty() {
		return errors.New("initial world transform config is empty")
	}

	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		c.sfs,
		headRef.GetTransformConf(),
	)
	if err != nil {
		return err
	}
	btx, bcs := block.NewTransaction(so.GetBlockStore(), xfrm, nil, nil)
	bcs.SetBlock(world_block.NewWorld(true), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		return err
	}
	headRef.RootRef = rootRef
	return nil
}

// processApplyTxOpWithEngine processes a ApplyTxOp operation with an existing engine.
func (c *Controller) processApplyTxOpWithEngine(
	ctx context.Context,
	le *logrus.Entry,
	txOp *ApplyTxOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
	ws *blkEngine,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Trace the application.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op")
	defer task.End()

	// Execute the transaction on the World and commit it.
	var nextRef *bucket.ObjectRef
	aerr := func() error {
		// Decode the transaction body.
		var ttx world_block_tx.Transaction
		{
			_, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/locate-tx")
			var err error
			ttx, err = txOp.GetTx().LocateTx()
			task.End()
			if err != nil {
				return err
			}
		}

		// Fork the head World for the replay.
		var btx *world_block.EngineTx
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/new-engine-tx")
			var err error
			btx, err = ws.bengine.NewBlockEngineTransaction(taskCtx, true)
			task.End()
			if err != nil {
				return err
			}
		}
		defer btx.Discard()

		// Apply the operations as the signing sender.
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/execute-tx")
			_, err := ttx.ExecuteTx(taskCtx, peerID, ws.lookupOp, btx)
			task.End()
			if err != nil {
				return err
			}
		}

		// Write the next World root.
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/commit")
		var err error
		nextRef, err = btx.CommitBlockTransaction(taskCtx)
		task.End()
		return err
	}()

	// A canceled context explains any failure.
	if ctx.Err() != nil {
		return nil, nil, context.Canceled
	}

	// A missing block is a local gap, not a property of the operation, so it
	// stops replay instead of rejecting.
	if errors.Is(aerr, block.ErrNotFound) {
		return nil, nil, aerr
	}

	// Reject a transaction that failed to apply.
	if aerr != nil {
		le.WithError(aerr).Warn("rejecting tx: apply failed")
		return nil, opRejection(peerID, nonce, "transaction apply failed: "+aerr.Error()), nil
	}

	// Accept it with the committed World as the head.
	nextHeadState := headState.CloneVT()
	nextHeadState.HeadRef = nextRef
	nextRef.BucketId = ""
	return nextHeadState, sobject.BuildSOOperationResult(peerID.String(), nonce, true, nil), nil
}
