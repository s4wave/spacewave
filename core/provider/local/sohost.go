package provider_local

import (
	"bytes"
	"context"
	"runtime/trace"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// LocalSOHost is the implementation of the local shared object host logic.
type LocalSOHost struct {
	// le is the logger
	le *logrus.Entry
	// privKey is the local private key
	privKey crypto.PrivKey
	// peerID is the local peer id
	peerID peer.ID
	// pubKey is the local public key
	pubKey []byte
	// objStore is the object store for local state.
	objStore object.ObjectStore
	// sharedObjectID is the ID of the shared object
	sharedObjectID string
	// sfs is the step factory set for transforms
	sfs *block_transform.StepFactorySet
	// queueOpCh is a channel to queue an operation to Execute.
	// once the value is received from the chan the result promise will be resolved.
	queueOpCh chan *queueOpTxn

	// below fields are managed by Execute.

	// soHost contains the stored SOState.
	soHost *sobject.SOHost
	// stateSnapCtr contains the current state snapshot
	stateSnapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
}

// queueOpTxn contains the txn to queue an operation.
type queueOpTxn struct {
	// op is the operation to queue
	op *sobject.QueuedSOOperation
	// done is closed when the txn is processed
	done chan struct{}
	// err contains the result
	// do not read until done is closed
	err error
}

// NewLocalSOHost constructs a new LocalSOHost.
func NewLocalSOHost(
	le *logrus.Entry,
	privKey crypto.PrivKey,
	soHost *sobject.SOHost,
	objStore object.ObjectStore,
	sharedObjectID string,
	sfs *block_transform.StepFactorySet,
) (*LocalSOHost, error) {
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	pubKey, err := crypto.MarshalPublicKey(privKey.GetPublic())
	if err != nil {
		return nil, err
	}

	return &LocalSOHost{
		le:             le,
		privKey:        privKey,
		peerID:         peerID,
		pubKey:         pubKey,
		objStore:       objStore,
		soHost:         soHost,
		sharedObjectID: sharedObjectID,
		sfs:            sfs,
		queueOpCh:      make(chan *queueOpTxn),
		stateSnapCtr:   ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
	}, nil
}

// Execute executes the LocalSOHost logic.
func (l *LocalSOHost) Execute(ctx context.Context) error {
	// Load the local state.
	localState, err := l.readLocalState(ctx)
	if err != nil {
		return err
	}

	// Get the state container
	stateCtr, relStateCtr, err := l.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	// Push the latest state into a channel.
	stateCh := make(chan *sobject.SOState, 1)
	go func() {
		var sstate *sobject.SOState
		var err error
		for {
			sstate, err = stateCtr.WaitValueChange(ctx, sstate, nil)
			if err != nil {
				// err is only returned here if ctx is canceled.
				return
			}
			select {
			case <-stateCh:
			default:
			}
			stateCh <- sstate
		}
	}()

	// Wait for initial state.
	var soState *sobject.SOState
	var snap sobject.SharedObjectStateSnapshot
	updateSnapshot := func() {
		snap = newLsoStateSnapshot(sobject.NewSOStateParticipantHandle(
			l.le,
			l.sfs,
			l.sharedObjectID,
			soState,
			l.privKey,
			l.peerID,
		), localState.CloneVT())
		l.stateSnapCtr.SetValue(snap)
	}

	processUpdatedSoState := func(updatedSoState *sobject.SOState) error {
		// Process rejections
		for _, peerRejections := range updatedSoState.GetOpRejections() {
			if peerRejections.GetPeerId() != l.peerID.String() {
				continue
			}
			for _, rejection := range peerRejections.GetRejections() {
				rejInner := &sobject.SOOperationRejectionInner{}
				if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
					l.le.WithError(err).Warn("failed to unmarshal rejection inner")
					return err
				}

				// Decode error details
				errorDetails, err := rejInner.DecodeErrorDetails(
					l.privKey,
					l.sharedObjectID,
					l.peerID,
				)
				if err != nil {
					l.le.WithError(err).Warn("failed to decode error details")
					return err
				}

				// Write the rejection to local state
				if err := l.writeLocalOpResult(ctx, &LocalSOOperationResult{
					LocalId: rejInner.GetLocalId(),
					Result: &sobject.SOOperationResult{
						OpRef: &sobject.SOOperationRef{
							PeerId: l.peerID.String(),
							Nonce:  rejInner.GetOpNonce(),
						},
						Body: &sobject.SOOperationResult_ErrorDetails{
							ErrorDetails: errorDetails,
						},
					},
				}); err != nil {
					l.le.WithError(err).Warn("failed to write operation result")
					return err
				}

				// Clear the rejection
				clearOp, err := sobject.BuildSOClearOperationResult(
					l.sharedObjectID,
					l.privKey,
					rejInner.GetLocalId(),
				)
				if err != nil {
					l.le.WithError(err).Warn("failed to build clear operation")
					return err
				}
				if err := l.soHost.ClearRejectedOperation(ctx, clearOp); err != nil {
					l.le.WithError(err).Warn("failed to clear rejected operation")
					return err
				}
			}
		}

		soState = updatedSoState
		updateSnapshot()
		return nil
	}

	select {
	case <-ctx.Done():
		return context.Canceled
	case soState = <-stateCh:
		updateSnapshot()
	}

	// Wait for something to happen:
	// - SOState is updated: update the localState
	// - We want to queue an op: queueOpCh => update localState => next loop transmit to remote SOHost.
	initial := true
	for {
		var queueOp *queueOpTxn

		if !initial {
			select {
			case <-ctx.Done():
				return context.Canceled
			case queueOp = <-l.queueOpCh:
				// Add operation to local state
				localState.OpQueue = append(localState.OpQueue, queueOp.op)
				err := l.writeLocalState(ctx, localState)
				queueOp.err = err
				if err != nil {
					close(queueOp.done)
					return err
				}
				updateSnapshot()
				close(queueOp.done)
			case updatedSoState := <-stateCh:
				if err := processUpdatedSoState(updatedSoState); err != nil {
					return err
				}
			}
		}
		initial = false

		// Process any queued local operations that need to be transmitted
		// The SOState will change after the first executeQueueOp, so just process one at a time here.
		if len(localState.OpQueue) != 0 {
			writeOp := localState.OpQueue[0]

			xfrm, err := snap.GetTransformer(ctx)
			if err != nil {
				return err
			}

			if err := l.executeQueueOp(ctx, xfrm, writeOp); err != nil {
				if ctx.Err() != nil {
					return context.Canceled
				}
				l.le.WithError(err).Warn("failed to queue operation to host")
				continue
			}

			// Remove the operation from the local queue
			localState.OpQueue[0] = nil
			localState.OpQueue = localState.OpQueue[1:]
			if err := l.writeLocalState(ctx, localState); err != nil {
				return err
			}

			// Expect a soState update after the txn was queued.
			// We must process this immediately so that the op doesn't disappear.
			select {
			case <-ctx.Done():
				return context.Canceled
			case updatedSoState := <-stateCh:
				if err := processUpdatedSoState(updatedSoState); err != nil {
					return err
				}
			}
		}
	}
}

// executeQueueOp is the part of Execute that queues ops against the remote SOHost.
func (l *LocalSOHost) executeQueueOp(
	ctx context.Context,
	xfrm *block_transform.Transformer,
	writeOp *sobject.QueuedSOOperation,
) error {
	// Make sure we don't already have a result.
	existingResult, err := l.readLocalOpResult(ctx, writeOp.GetLocalId())
	if err != nil {
		return err
	}
	if existingResult != nil {
		// already processed, no-op
		return nil
	}

	// Encode the operation.
	encOpData, err := xfrm.EncodeBlock(writeOp.GetOpData())
	if err != nil {
		return err
	}

	// Queue the operation.
	qerr := l.soHost.QueueOperation(ctx, l.peerID, func(nonce uint64) (*sobject.SOOperation, error) {
		return sobject.BuildSOOperation(
			l.soHost.GetSharedObjectID(),
			l.privKey,
			encOpData,
			nonce,
			writeOp.GetLocalId(),
		)
	})
	if qerr != nil {
		// ignore the error if ctx was canceled
		if ctx.Err() != nil {
			return context.Canceled
		}
		// otherwise mark the op as rejected.
		werr := l.writeLocalOpResult(context.Background(), &LocalSOOperationResult{
			LocalId: writeOp.GetLocalId(),
			Result: &sobject.SOOperationResult{
				OpRef: &sobject.SOOperationRef{
					PeerId: l.peerID.String(),
					Nonce:  0,
				},
				Body: &sobject.SOOperationResult_ErrorDetails{
					ErrorDetails: &sobject.SOOperationRejectionErrorDetails{
						ErrorMsg: qerr.Error(),
					},
				},
			},
		})
		if werr != nil {
			return werr
		}
	}

	return nil
}

// AccessSharedObjectState adds a reference to the state and returns the state container.
func (l *LocalSOHost) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return l.stateSnapCtr, func() {}, nil
}

// QueueOperation applies an operation to the shared object op queue.
// Returns after the operation is applied to the local queue.
// Returns the local op id.
func (l *LocalSOHost) QueueOperation(ctx context.Context, op []byte) (string, error) {
	ctx, task := trace.NewTask(ctx, "alpha/local-so/queue-operation")
	defer task.End()

	id := sobject.NewSOOperationLocalID()
	done := make(chan struct{})

	txn := &queueOpTxn{
		op: &sobject.QueuedSOOperation{
			LocalId: id,
			OpData:  op,
		},
		done: done,
	}

	{
		taskCtx, task := trace.NewTask(ctx, "alpha/local-so/queue-operation/enqueue")
		select {
		case <-taskCtx.Done():
			task.End()
			return "", context.Canceled
		case l.queueOpCh <- txn:
		}
		task.End()
	}

	// wait for processing
	{
		_, task := trace.NewTask(ctx, "alpha/local-so/queue-operation/wait")
		<-txn.done
		task.End()
	}
	if err := txn.err; err != nil {
		return "", err
	}
	return id, nil
}

// WaitOperation waits for the operation to be confirmed or rejected by the provider.
// Returns the current state nonce (greater than or equal to the nonce when the op was applied).
// After ClearOperation has been called, this will return success even for failed ops!
// If the operation was rejected, returns 0, true, error.
// Any other error returns 0, false, error
func (l *LocalSOHost) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	soStateCtr, relSoStateCtr, err := l.soHost.GetSOStateCtr(ctx, ctxCancel)
	if err != nil {
		return 0, false, err
	}
	defer relSoStateCtr()

	var current *sobject.SOState
	for {
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return 0, false, err
		}
		current = next

		// Look for the operation ID in our queue.
		var queuedOp *sobject.SOOperation
		for _, op := range current.GetOps() {
			// Not our operation.
			if !bytes.Equal(op.GetSignature().GetPubKey(), l.pubKey) {
				continue
			}

			// Unmarshal inner.
			opInner, err := op.UnmarshalInner()
			if err != nil {
				return 0, false, err
			}

			// Check if match
			opInnerLocalID := opInner.GetLocalId()
			if opInnerLocalID == localID {
				queuedOp = op
				break
			}
		}

		// If queuedOp, then we are still waiting for the op to be applied.
		if queuedOp != nil {
			continue
		}

		// Check if there is a local op result.
		localOpResult, err := l.readLocalOpResult(ctx, localID)
		if err != nil {
			return 0, false, err
		}
		if localOpResult.GetLocalId() == localID {
			errorMsg := localOpResult.GetResult().GetErrorDetails().GetErrorMsg()
			if errorMsg != "" {
				// Error
				return 0, true, errors.Wrap(sobject.ErrRejectedOp, errorMsg)
			}
		}

		// Check if there is a rejection.
		for _, peerRejections := range current.GetOpRejections() {
			if peerRejections.GetPeerId() != l.peerID.String() {
				continue
			}
			for _, rejection := range peerRejections.GetRejections() {
				rejInner := &sobject.SOOperationRejectionInner{}
				if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
					return 0, false, err
				}
				if rejInner.GetLocalId() == localID {
					errorDetails, err := rejInner.DecodeErrorDetails(l.privKey, l.soHost.GetSharedObjectID(), l.peerID)
					if err != nil {
						return 0, false, err
					}
					if len(errorDetails.GetErrorMsg()) == 0 {
						return 0, true, sobject.ErrRejectedOp
					}
					return 0, true, errors.Wrap(sobject.ErrRejectedOp, errorDetails.GetErrorMsg())
				}
			}
		}

		// Operation is not in the queue and not rejected, so it must have been applied.
		// Get the state snapshot
		snap := sobject.NewSOStateParticipantHandle(
			l.le,
			l.sfs,
			l.sharedObjectID,
			current,
			l.privKey,
			l.peerID,
		)

		// Use snapshot to decode root inner
		rootInner, err := snap.GetRootInner(ctx)
		if err != nil {
			return 0, false, err
		}
		if rootInner == nil {
			return 0, false, errors.New("root inner state is nil")
		}
		return rootInner.GetSeqno(), false, nil
	}
}

// readLocalState reads the local state from the object store.
func (l *LocalSOHost) readLocalState(ctx context.Context) (*LocalSOState, error) {
	localStateKey := SobjectObjectStoreLocalStateKey(l.soHost.GetSharedObjectID())
	tx, err := l.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	localStateData, found, err := tx.Get(ctx, localStateKey)
	if err != nil {
		return nil, err
	}

	lstate := &LocalSOState{}
	if found {
		if err := lstate.UnmarshalVT(localStateData); err != nil {
			return nil, err
		}
	}
	return lstate, nil
}

// writeLocalState writes the local state to the object store.
func (l *LocalSOHost) writeLocalState(ctx context.Context, next *LocalSOState) error {
	ctx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state")
	defer task.End()

	localStateKey := SobjectObjectStoreLocalStateKey(l.soHost.GetSharedObjectID())
	var tx kvtx.Tx
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/new-transaction")
		var err error
		tx, err = l.objStore.NewTransaction(taskCtx, true)
		task.End()
		if err != nil {
			return err
		}
	}
	defer tx.Discard()

	var data []byte
	{
		_, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/marshal")
		var err error
		data, err = next.MarshalVT()
		task.End()
		if err != nil {
			return err
		}
	}
	defer scrub.Scrub(data)

	{
		taskCtx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/set")
		err := tx.Set(taskCtx, localStateKey, data)
		task.End()
		if err != nil {
			return err
		}
	}

	{
		taskCtx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/commit")
		err := tx.Commit(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}
	return nil
}

// readLocalOpResult reads the operation result from the object store.
func (l *LocalSOHost) readLocalOpResult(ctx context.Context, localOpID string) (*LocalSOOperationResult, error) {
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), localOpID)
	tx, err := l.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	opResultData, found, err := tx.Get(ctx, opResultKey)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	opResult := &LocalSOOperationResult{}
	if err := opResult.UnmarshalVT(opResultData); err != nil {
		return nil, err
	}
	if opResult.GetLocalId() != localOpID {
		return nil, errors.New("read result from storage has wrong local id")
	}

	return opResult, nil
}

// writeLocalOpResult writes the operation result to the object store.
func (l *LocalSOHost) writeLocalOpResult(ctx context.Context, result *LocalSOOperationResult) error {
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), result.GetLocalId())
	tx, err := l.objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	data, err := result.MarshalVT()
	if err != nil {
		return err
	}
	defer scrub.Scrub(data)

	if err := tx.Set(ctx, opResultKey, data); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// clearLocalOpResult clears the operation result from the object store.
func (l *LocalSOHost) clearLocalOpResult(ctx context.Context, localID string) error {
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), localID)
	tx, err := l.objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if err := tx.Delete(ctx, opResultKey); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
