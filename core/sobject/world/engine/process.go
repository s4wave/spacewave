package sobject_world_engine

import (
	"context"
	"runtime/trace"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// processOp processes a single operation and returns the next state and operation result.
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
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/process-op")
	defer task.End()

	op := &SOWorldOp{}
	if err := op.UnmarshalVT(opData); err != nil {
		if peerID != "" {
			return nil, sobject.BuildSOOperationResult(
				peerID.String(),
				nonce,
				false,
				&sobject.SOOperationRejectionErrorDetails{
					ErrorMsg: "invalid operation data: " + err.Error(),
				},
			), nil
		}
		return nil, nil, err
	}

	ole := le.WithFields(logrus.Fields{
		"op-idx":      opIdx,
		"op-local-id": localID,
		"op-nonce":    nonce,
		"op-peer-id":  peerID.String(),
	})
	ole.Debug("processing op")

	switch body := op.GetBody().(type) {
	case *SOWorldOp_InitWorld:
		return c.processInitWorldOp(
			ole,
			body.InitWorld,
			headState,
			peerID,
			nonce,
		)
	case *SOWorldOp_ApplyTxOp:
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

		if headState.GetHeadRef().GetEmpty() {
			if peerID != "" {
				ole.Warn("rejecting apply tx op: world is not initialized")
				return nil, sobject.BuildSOOperationResult(
					peerID.String(),
					nonce,
					false,
					&sobject.SOOperationRejectionErrorDetails{
						ErrorMsg: "world is not initialized",
					},
				), nil
			}
			return nil, nil, errors.New("world is not initialized")
		}

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
	default:
		ole.Warn("rejecting op: unknown op type")
		return nil, sobject.BuildSOOperationResult(
			peerID.String(),
			nonce,
			false,
			&sobject.SOOperationRejectionErrorDetails{
				ErrorMsg: "unknown operation type",
			},
		), nil
	}
}

// processInitWorldOp processes an InitWorld operation.
func (c *Controller) processInitWorldOp(
	le *logrus.Entry,
	initOp *InitWorldOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Only allow init if there's no existing state
	if !headState.GetHeadRef().GetEmpty() {
		le.Warn("rejecting world init op: world is already initialized")
		return nil, sobject.BuildSOOperationResult(
			peerID.String(),
			nonce,
			false,
			&sobject.SOOperationRejectionErrorDetails{
				ErrorMsg: "world is already initialized",
			},
		), nil
	}

	// Generate transform config if not provided
	transformConf := initOp.GetTransformConf()
	if transformConf.GetEmpty() {
		var err error
		transformConf, err = buildDefaultTransformConf()
		if err != nil {
			return nil, nil, err
		}
	}

	// Create the op result (accept)
	opResult := sobject.BuildSOOperationResult(
		peerID.String(),
		nonce,
		true,
		nil,
	)

	// TODO: initialize the root ref with an empty world block?

	// Create the mutated result state
	finalState := &InnerState{
		HeadRef: &bucket.ObjectRef{
			TransformConf: transformConf,
		},
	}

	return finalState, opResult, nil
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
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op")
	defer task.End()

	var nextRef *bucket.ObjectRef
	aerr := func() error {
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

		{
			taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/execute-tx")
			_, err := ttx.ExecuteTx(taskCtx, peerID, ws.lookupOp, btx)
			task.End()
			if err != nil {
				return err
			}
		}

		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/process-apply-tx-op/commit")
		var err error
		nextRef, err = btx.CommitBlockTransaction(taskCtx)
		task.End()
		return err
	}()

	// if context canceled ignore error
	if ctx.Err() != nil {
		return nil, nil, context.Canceled
	}

	// If applying the transaction failed
	if aerr != nil {
		le.WithError(aerr).Warn("rejecting tx: apply failed")
		return nil, sobject.BuildSOOperationResult(
			peerID.String(),
			nonce,
			false,
			&sobject.SOOperationRejectionErrorDetails{
				ErrorMsg: "transaction apply failed: " + aerr.Error(),
			},
		), nil
	}

	// Update the head state
	nextHeadState := headState.CloneVT()
	nextHeadState.HeadRef = nextRef
	nextRef.BucketId = ""

	return nextHeadState, sobject.BuildSOOperationResult(
		peerID.String(),
		nonce,
		true,
		nil,
	), nil
}
