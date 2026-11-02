package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// processOp processes a single operation on w and returns the next state and
// operation result. An operation with no peerID is local, and its rejection
// is returned as an error.
func (c *Controller) processOp(
	ctx context.Context,
	le *logrus.Entry,
	w *replayWorld,
	opData []byte,
	localID string,
	peerID peer.ID,
	nonce uint64,
	opIdx int,
	headState *InnerState,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Trace the operation through its result.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/process-op")
	defer task.End()

	// Decode the World operation.
	_, decodeTask := trace.NewTask(ctx, "alpha/so-engine/process-op/decode")
	op := &SOWorldOp{}
	err := op.UnmarshalVT(opData)
	decodeTask.End()
	if err != nil {
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
			w.store,
			body.InitWorld,
			headState,
			peerID,
			nonce,
		)
	case *SOWorldOp_ApplyTxOp:
		if headState.GetHeadRef().GetEmpty() {
			return rejectOp(ole, peerID, nonce, "world is not initialized")
		}

		// Bind the head World.
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-op/bind-world")
		ws, err := w.bind(taskCtx, headState.GetHeadRef())
		task.End()
		if err != nil {
			return nil, nil, err
		}

		// Apply the transaction to it.
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
		if nhs != nil {
			w.advance(nhs.GetHeadRef())
			ole.Debugf("applied world txn op: %v", body.ApplyTxOp.GetTx().GetTxType().String())
		}
		return nhs, res, nil
	case *SOWorldOp_SetRetainedRoot:
		return processSetRetainedRootOp(ole, body.SetRetainedRoot, headState, peerID, nonce)
	case *SOWorldOp_SetTransformConf:
		return processSetTransformConfOp(ole, body.SetTransformConf, headState, peerID, nonce)
	case *SOWorldOp_GraphSettingsStep:
		stepOp := body.GraphSettingsStep
		if headState.GetHeadRef().GetEmpty() {
			return rejectOp(ole, peerID, nonce, "world is not initialized")
		}
		if err := validateGraphSettingsStep(stepOp.GetTarget(), int(stepOp.GetMax())); err != nil {
			return rejectOp(ole, peerID, nonce, err.Error())
		}

		// Step the head World.
		ws, err := w.bind(ctx, headState.GetHeadRef())
		if err != nil {
			return nil, nil, err
		}
		nhs, res, err := processGraphSettingsStepOp(ctx, ole, stepOp, headState, peerID, nonce, ws)
		if err != nil {
			return nil, nil, err
		}
		if nhs != nil && !nhs.GetHeadRef().EqualVT(headState.GetHeadRef()) {
			w.advance(nhs.GetHeadRef())
		}
		return nhs, res, nil
	default:
		ole.Warn("rejecting op: unknown op type")
		return nil, opRejection(peerID, nonce, "unknown operation type"), nil
	}
}

// opPayloadRefs returns the payload roots of the World operations in the
// encoded operation, resolving operation types with lookupOp.
func opPayloadRefs(ctx context.Context, lookupOp world.LookupOp, opData []byte) ([]*block.BlockRef, error) {
	// Decode the operation.
	op := &SOWorldOp{}
	if err := op.UnmarshalVT(opData); err != nil {
		return nil, err
	}

	// Only a World transaction carries payloads.
	tx := op.GetApplyTxOp().GetTx()
	if tx == nil {
		return nil, nil
	}
	return tx.PayloadRefs(ctx, lookupOp)
}

// processInitWorldOp processes an InitWorld operation.
func (c *Controller) processInitWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	store bstore.BlockStore,
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
	if err := c.writeInitialWorldRoot(ctx, le, store, initOp, finalState.GetHeadRef()); err != nil {
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

// writeInitialWorldRoot writes an initial World to store when changelog
// initialization is disabled or the graph starts with settings. A page view
// keeps the root as a World page, as a commit keeps its pages.
func (c *Controller) writeInitialWorldRoot(
	ctx context.Context,
	le *logrus.Entry,
	store bstore.BlockStore,
	initOp *InitWorldOp,
	headRef *bucket.ObjectRef,
) error {
	// Only a World without a changelog or with graph settings starts from a
	// written root.
	settings := initOp.GetGraphSettings()
	if !initOp.GetLastChangeDisable() && len(settings.GetIndexes()) == 0 {
		return nil
	}
	if store == nil {
		return errors.New("block store is required to write the initial world root")
	}
	if headRef.GetTransformConf().GetEmpty() {
		return errors.New("initial world transform config is empty")
	}

	// Write the empty World root under the head's transform.
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		c.sfs,
		headRef.GetTransformConf(),
	)
	if err != nil {
		return err
	}

	// Write the root holding the World's starting settings.
	write := func(ops block.StoreOps) error {
		// Build the root and write it.
		btx, bcs := block.NewTransaction(ops, xfrm, nil, nil)
		root := world_block.NewWorld(initOp.GetLastChangeDisable())
		root.GraphSettings = settings.CloneVT()
		bcs.SetBlock(root, true)
		rootRef, _, err := btx.Write(ctx, true)
		headRef.RootRef = rootRef
		return err
	}
	if pages, ok := store.(*worldPages); ok {
		return pages.write(ctx, write)
	}
	return write(store)
}

// processApplyTxOpWithEngine processes an ApplyTxOp operation with an existing engine.
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
			_, err := ttx.ExecuteTx(taskCtx, peerID, c.declaredLookup(ws.lookupOp, btx), btx)
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

	// End the pass when its own context ends.
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// A dependency's canceled read can outlive its transport while this pass
	// stays live. Like an unavailable block or handler, it stops replay:
	// rejecting the operation would make a local failure split replicas.
	if isReplayGap(aerr) || errors.Is(aerr, context.Canceled) || errors.Is(aerr, context.DeadlineExceeded) {
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
