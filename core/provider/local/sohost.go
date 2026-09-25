package provider_local

import (
	"bytes"
	"context"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// LocalSOHost is the implementation of the local shared object host logic.
type LocalSOHost struct {
	// le is the logger.
	le *logrus.Entry
	// privKey is the local private key.
	privKey crypto.PrivKey
	// peerID is the local peer ID.
	peerID peer.ID
	// pubKey is the local public key.
	pubKey []byte
	// objStore is the object store for local state.
	objStore object.ObjectStore
	// sharedObjectID is the ID of the shared object.
	sharedObjectID string
	// sfs is the step factory set for transforms.
	sfs *block_transform.StepFactorySet
	// queueOpCh is a channel to queue an operation to Execute.
	// Execute closes the transaction's done channel after persisting its result.
	queueOpCh chan *queueOpTxn

	// soHost contains the stored SOState.
	soHost *sobject.SOHost
	// stateSnapCtr contains the body snapshot published by Execute.
	stateSnapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	// publishedConfigCtr acknowledges the configuration represented by stateSnapCtr.
	publishedConfigCtr *ccontainer.CContainer[*sobject.SharedObjectConfig]
	// validator is the local validator's processing function while it runs.
	// Transmission validates queued operations with it in the queue write.
	validator atomic.Pointer[sobject.ProcessOpsFunc]
}

// queueOpTxn contains the txn to queue an operation.
type queueOpTxn struct {
	// op is the operation to queue.
	op *sobject.QueuedSOOperation
	// done is closed when the transaction is processed.
	done chan struct{}
	// err contains the result and must not be read until done is closed.
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
	// Derive the participant identity used for signatures and operation matching.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	pubKey, err := crypto.MarshalPublicKey(privKey.GetPublic())
	if err != nil {
		return nil, err
	}

	// Create the operation channel and publication containers for Execute.
	return &LocalSOHost{
		le:                 le,
		privKey:            privKey,
		peerID:             peerID,
		pubKey:             pubKey,
		objStore:           objStore,
		soHost:             soHost,
		sharedObjectID:     sharedObjectID,
		sfs:                sfs,
		queueOpCh:          make(chan *queueOpTxn),
		stateSnapCtr:       ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
		publishedConfigCtr: ccontainer.NewCContainer[*sobject.SharedObjectConfig](nil),
	}, nil
}

// Execute executes the LocalSOHost logic.
func (l *LocalSOHost) Execute(ctx context.Context) error {
	// Load the local state.
	localState, err := l.readLocalState(ctx)
	if err != nil {
		return err
	}

	// Retain the accepted host-state watch for the execution lifetime.
	stateCtr, relStateCtr, err := l.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	// Push the latest state into a channel.
	stateCh := make(chan *sobject.SOState, 1)
	go func() {
		// Coalesce host updates while retaining the newest accepted state.
		var sstate *sobject.SOState
		var err error
		for {
			// End forwarding when the host watch's context is canceled.
			sstate, err = stateCtr.WaitValueChange(ctx, sstate, nil)
			if err != nil {
				return
			}

			// Replace the queued snapshot before handing it to Execute.
			select {
			case <-stateCh:
			default:
			}
			stateCh <- sstate
		}
	}()

	// Publish immutable body and configuration snapshots from accepted host state.
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
		l.publishedConfigCtr.SetValue(soState.GetConfig().CloneVT())
	}

	// Persist operation outcomes only after their accepted snapshot is readable.
	processUpdatedSoState := func(updatedSoState *sobject.SOState) error {
		// Advance the snapshot before publishing operation completion.
		prevSoState := soState
		soState = updatedSoState
		updateSnapshot()

		// WaitOperation can resolve from the persisted local op result. Publish
		// the accepted snapshot first so callers that immediately read state do
		// not observe the pre-acceptance snapshot.
		if err := l.writeAcceptedLocalOpResults(ctx, prevSoState, updatedSoState); err != nil {
			l.le.WithError(err).Warn("failed to write accepted operation result")
			return err
		}

		// Persist this participant's rejections before clearing them on the host.
		for _, peerRejections := range updatedSoState.GetOpRejections() {
			// Ignore results addressed to other participants.
			if peerRejections.GetPeerId() != l.peerID.String() {
				continue
			}

			// Decode each rejection under the validator's signing identity.
			for _, rejection := range peerRejections.GetRejections() {
				// Decode the signed rejection envelope.
				rejInner := &sobject.SOOperationRejectionInner{}
				if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
					l.le.WithError(err).Warn("failed to unmarshal rejection inner")
					return err
				}

				// Decode error details using the validator signer identity.
				errorDetails, err := l.decodeLocalRejectionError(rejection, rejInner)
				if err != nil {
					l.le.WithError(err).Warn("failed to decode error details")
					return err
				}

				// Preserve the rejection in local storage for WaitOperation.
				if err := l.writeLocalOpResult(ctx, &LocalSOOperationResult{
					LocalId:   rejInner.GetLocalId(),
					RootSeqno: updatedSoState.GetRoot().GetInnerSeqno(),
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

				// Clear the host rejection after its local record is durable.
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
		return nil
	}

	// Consume the host update produced by queueing an operation on the host.
	// Processing it at once keeps the operation visible in the snapshot.
	awaitHostUpdate := func() error {
		select {
		case <-ctx.Done():
			return context.Canceled
		case updatedSoState := <-stateCh:
			return processUpdatedSoState(updatedSoState)
		}
	}

	// Publish initial host state before accepting local operations.
	select {
	case <-ctx.Done():
		return context.Canceled
	case soState = <-stateCh:
		updateSnapshot()
	}

	// Serialize local queue writes with publication of accepted host updates.
	initial := true
	for {
		// Drain the initial durable queue before waiting for new work.
		var queueOp *queueOpTxn
		if !initial {
			select {
			case <-ctx.Done():
				return context.Canceled
			case queueOp = <-l.queueOpCh:
				// With nothing queued ahead of it, transmit the operation
				// directly: the host queue commit is as durable as the local
				// queue commit it replaces. The caller is released once the
				// operation is visible in the snapshot.
				if len(localState.OpQueue) == 0 {
					xfrm, err := snap.GetTransformer(ctx)
					if err != nil {
						queueOp.err = err
						close(queueOp.done)
						return err
					}
					queued, err := l.executeQueueOp(ctx, xfrm, queueOp.op)
					if err == nil {
						if queued {
							err = awaitHostUpdate()
						}
						close(queueOp.done)
						if err != nil {
							return err
						}
						continue
					}
					if ctx.Err() != nil {
						queueOp.err = context.Canceled
						close(queueOp.done)
						return context.Canceled
					}
					l.le.WithError(err).Warn("failed to queue operation to host")
				}

				// Persist the operation before acknowledging its local queue entry.
				localState.OpQueue = append(localState.OpQueue, queueOp.op)
				err := l.writeLocalState(ctx, localState)
				queueOp.err = err
				if err != nil {
					close(queueOp.done)
					return err
				}

				// Make the durable queue entry visible before releasing its caller.
				updateSnapshot()
				close(queueOp.done)
			case updatedSoState := <-stateCh:
				if err := processUpdatedSoState(updatedSoState); err != nil {
					return err
				}
			}
		}
		initial = false

		// Transmit one queued operation per host update so its snapshot stays current.
		if len(localState.OpQueue) != 0 {
			// Resolve the transform from the snapshot that owns the queued operation.
			writeOp := localState.OpQueue[0]
			xfrm, err := snap.GetTransformer(ctx)
			if err != nil {
				return err
			}

			// Transmit one operation before consuming its resulting host update.
			queued, err := l.executeQueueOp(ctx, xfrm, writeOp)
			if err != nil {
				if ctx.Err() != nil {
					return context.Canceled
				}
				l.le.WithError(err).Warn("failed to queue operation to host")
				continue
			}

			// Remove the transmitted operation from the durable local queue.
			localState.OpQueue[0] = nil
			localState.OpQueue = localState.OpQueue[1:]
			if err := l.writeLocalState(ctx, localState); err != nil {
				return err
			}

			// A resolved operation left the host state unchanged; publish the
			// shorter queue. A queued one publishes with its host update.
			if !queued {
				updateSnapshot()
				continue
			}
			if err := awaitHostUpdate(); err != nil {
				return err
			}
		}
	}
}

// executeQueueOp transmits a queued operation to the SOHost. queued reports
// whether this call added it to the host queue, which produces a host update.
// A nil error with queued unset means the operation already has a durable
// result: an earlier attempt's, or the host's rejection recorded here. An
// error leaves the operation for a later attempt.
func (l *LocalSOHost) executeQueueOp(
	ctx context.Context,
	xfrm *block_transform.Transformer,
	writeOp *sobject.QueuedSOOperation,
) (bool, error) {
	// Preserve any outcome recovered from an earlier transmission attempt.
	existingResult, err := l.readLocalOpResult(ctx, writeOp.GetLocalId())
	if err != nil {
		return false, err
	}
	if existingResult != nil {
		return false, nil
	}

	// Encode the operation.
	encOpData, err := xfrm.EncodeBlock(writeOp.GetOpData())
	if err != nil {
		return false, err
	}

	// Queue the operation, validating it in the same write when the local
	// validator runs.
	qerr := l.soHost.QueueOperationAndProcess(ctx, l.peerID, func(nonce uint64) (*sobject.SOOperation, error) {
		return sobject.BuildSOOperation(
			l.soHost.GetSharedObjectID(),
			l.privKey,
			encOpData,
			nonce,
			writeOp.GetLocalId(),
		)
	}, l.queuedOpsProcessor())
	if qerr == nil {
		return true, nil
	}

	// Leave canceled transmissions in the durable queue for the next execution.
	if ctx.Err() != nil {
		return false, context.Canceled
	}

	// Persist a terminal queue rejection so WaitOperation can report it.
	return false, l.writeLocalOpResult(context.Background(), &LocalSOOperationResult{
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
}

// setValidator registers cb as the local validator until the returned
// function is called.
func (l *LocalSOHost) setValidator(cb sobject.ProcessOpsFunc) func() {
	ptr := &cb
	l.validator.Store(ptr)
	return func() { l.validator.CompareAndSwap(ptr, nil) }
}

// queuedOpsProcessor returns a processor that runs the local validator on a
// locked host state, or nil if no local validator runs.
func (l *LocalSOHost) queuedOpsProcessor() sobject.QueuedOpsProcessor {
	cb := l.validator.Load()
	if cb == nil {
		return nil
	}
	return func(ctx context.Context, state *sobject.SOState) (*sobject.SORoot, []*sobject.SOOperationRejection, []*sobject.SOOperation, error) {
		snap := sobject.NewSOStateParticipantHandle(l.le, l.sfs, l.sharedObjectID, state, l.privKey, l.peerID)
		return snap.ProcessOperations(ctx, state.GetOps(), func(ctx context.Context, data []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
			return (*cb)(ctx, snap, data, ops)
		})
	}
}

// waitPublishedConfig waits until body readers can observe target or a verified descendant.
func (l *LocalSOHost) waitPublishedConfig(ctx context.Context, target *sobject.SharedObjectConfig) error {
	// Require a target and the publication container that acknowledges it.
	if target == nil || l.publishedConfigCtr == nil {
		return errors.New("published SharedObject configuration is unavailable")
	}

	// Accept the target configuration or a descendant with verified history.
	_, err := l.publishedConfigCtr.WaitValueWithValidator(ctx, func(current *sobject.SharedObjectConfig) (bool, error) {
		// Keep waiting until the published configuration reaches the target.
		if current == nil || current.GetConfigChainSeqno() < target.GetConfigChainSeqno() {
			return false, nil
		}
		if current.EqualVT(target) {
			return true, nil
		}

		// Verify a newer configuration descends from the requested target.
		changes, err := l.soHost.ReadConfigHistory(ctx, target.GetConfigChainHash(), current.GetConfigChainHash())
		if err != nil {
			return false, err
		}
		if err := sobject.VerifyConfigChainSuffix(target, current, changes); err != nil {
			return false, err
		}
		return true, nil
	}, nil)
	return err
}

// AccessSharedObjectState adds a reference to the state and returns the state container.
func (l *LocalSOHost) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return l.stateSnapCtr, func() {}, nil
}

// QueueOperation applies an operation to the shared object op queue.
// Returns after the operation is durable in the host queue, the local queue,
// or as a recorded rejection, and visible in the published snapshot.
// Returns the local op id.
func (l *LocalSOHost) QueueOperation(ctx context.Context, op []byte) (string, error) {
	// Trace one local enqueue through its persistence acknowledgement.
	ctx, task := trace.NewTask(ctx, "alpha/local-so/queue-operation")
	defer task.End()

	// Identify the durable operation and its completion channel.
	id := sobject.NewSOOperationLocalID()
	done := make(chan struct{})
	txn := &queueOpTxn{
		op: &sobject.QueuedSOOperation{
			LocalId: id,
			OpData:  op,
		},
		done: done,
	}

	// Hand the operation to Execute unless the caller cancels first.
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

	// Once accepted by Execute, report its definitive persistence result.
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

// writeAcceptedLocalOpResults records operations removed by an accepted host update.
func (l *LocalSOHost) writeAcceptedLocalOpResults(
	ctx context.Context,
	prevState *sobject.SOState,
	updatedState *sobject.SOState,
) error {
	// Initial state has no earlier operation queue to reconcile.
	if prevState == nil || updatedState == nil {
		return nil
	}

	// Index local operations that remain pending in the accepted state.
	pendingLocalIDs := make(map[string]struct{})
	for _, op := range updatedState.GetOps() {
		// Decode only operations signed by the local participant.
		inner, ok, err := l.localOperationInner(op)
		if err != nil {
			return err
		}
		if ok && inner.GetLocalId() != "" {
			pendingLocalIDs[inner.GetLocalId()] = struct{}{}
		}
	}

	// Index local rejections so queue removal cannot become a false success.
	rejectedLocalIDs := make(map[string]struct{})
	for _, peerRejections := range updatedState.GetOpRejections() {
		// Restrict the rejection set to this participant.
		if peerRejections.GetPeerId() != l.peerID.String() {
			continue
		}

		// Decode the rejected operation identifiers from their signed envelopes.
		for _, rejection := range peerRejections.GetRejections() {
			rejInner := &sobject.SOOperationRejectionInner{}
			if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
				return err
			}
			if rejInner.GetLocalId() != "" {
				rejectedLocalIDs[rejInner.GetLocalId()] = struct{}{}
			}
		}
	}

	// Persist success for local operations that left the queue without rejection.
	for _, op := range prevState.GetOps() {
		// Select completed local operations with a caller-visible identifier.
		inner, ok, err := l.localOperationInner(op)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		localID := inner.GetLocalId()
		if localID == "" {
			continue
		}
		if _, pending := pendingLocalIDs[localID]; pending {
			continue
		}
		if _, rejected := rejectedLocalIDs[localID]; rejected {
			continue
		}

		// Preserve an outcome already recorded by an earlier execution.
		existingResult, err := l.readLocalOpResult(ctx, localID)
		if err != nil {
			return err
		}
		if existingResult != nil {
			continue
		}

		// Record the root sequence whose publication completed this operation.
		if err := l.writeAcceptedLocalOpResult(ctx, &LocalSOOperationResult{
			LocalId:   localID,
			RootSeqno: updatedState.GetRoot().GetInnerSeqno(),
			Result: sobject.BuildSOOperationResult(
				l.peerID.String(),
				inner.GetNonce(),
				true,
				nil,
			),
		}); err != nil {
			return err
		}
	}

	return nil
}

// localOperationInner decodes an operation signed by the local participant.
func (l *LocalSOHost) localOperationInner(
	op *sobject.SOOperation,
) (*sobject.SOOperationInner, bool, error) {
	// Reject absent operations and signatures from other participants.
	if op == nil || !bytes.Equal(op.GetSignature().GetPubKey(), l.pubKey) {
		return nil, false, nil
	}

	// Decode the matching operation's signed envelope.
	inner, err := op.UnmarshalInner()
	if err != nil {
		return nil, false, err
	}
	return inner, true, nil
}

// WaitOperation waits for the operation to be confirmed or rejected by the provider.
// Success waits until body readers can observe at least the accepted root nonce.
// Returns the nonce of that published snapshot.
// After ClearOperation has been called, this will return success even for failed ops!
// If the operation was rejected, returns 0, true, error.
// Any other error returns 0, false, error.
func (l *LocalSOHost) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	// Recover a persisted outcome and fence successful results against body readers.
	if seqno, rejected, err, resolved := l.localOpResultOutcome(ctx, localID); err != nil || resolved {
		if err == nil && resolved && !rejected && l.soHost.CanWatchSOState() {
			seqno, err = l.waitForRootSeqno(ctx, seqno)
		}
		return seqno, rejected, err
	}

	// Keep locally queued operations pending until transmission to the host.
	if err := l.waitForLocalOperationTransmission(ctx, localID); err != nil {
		return 0, false, err
	}

	// Transmission may persist a rejection or discover an existing acceptance.
	if seqno, rejected, err, resolved := l.localOpResultOutcome(ctx, localID); err != nil || resolved {
		if err == nil && resolved && !rejected && l.soHost.CanWatchSOState() {
			seqno, err = l.waitForRootSeqno(ctx, seqno)
		}
		return seqno, rejected, err
	}

	// Retain host state while resolving acceptance or rejection of the operation.
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()
	soStateCtr, relSoStateCtr, err := l.soHost.GetSOStateCtr(ctx, ctxCancel)
	if err != nil {
		return 0, false, err
	}
	defer relSoStateCtr()

	// Follow host transitions until this operation leaves its queue.
	var current *sobject.SOState
	for {
		// Read each host snapshot once, retaining the wait across unchanged state.
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return 0, false, err
		}
		current = next

		// Look for the operation ID in our queue.
		var queuedOp *sobject.SOOperation
		for _, op := range current.GetOps() {
			// Ignore operations signed by other participants.
			if !bytes.Equal(op.GetSignature().GetPubKey(), l.pubKey) {
				continue
			}

			// Decode the participant's signed operation envelope.
			opInner, err := op.UnmarshalInner()
			if err != nil {
				return 0, false, err
			}

			// Retain the matching operation while it remains pending.
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
		if seqno, rejected, err, resolved := l.localOpResultOutcome(ctx, localID); err != nil || resolved {
			if err == nil && resolved && !rejected {
				seqno, err = l.waitForRootSeqno(ctx, seqno)
			}
			return seqno, rejected, err
		}

		// Check if there is a rejection.
		for _, peerRejections := range current.GetOpRejections() {
			// Ignore rejection records for other participants.
			if peerRejections.GetPeerId() != l.peerID.String() {
				continue
			}

			// Find the rejection corresponding to this caller's operation.
			for _, rejection := range peerRejections.GetRejections() {
				// Decode the rejection's operation identity.
				rejInner := &sobject.SOOperationRejectionInner{}
				if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
					return 0, false, err
				}

				// Report the matching rejection using its stored error details.
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

		// Acceptance must reach the snapshot used by GetSharedObjectState before
		// the caller can treat the operation as complete.
		seqno, err := l.waitForRootSeqno(ctx, current.GetRoot().GetInnerSeqno())
		return seqno, false, err
	}
}

// waitForLocalOperationTransmission waits until the durable local queue no longer
// contains the operation. Hosts without local snapshots only resolve stored results.
func (l *LocalSOHost) waitForLocalOperationTransmission(ctx context.Context, localID string) error {
	// A result-only host has no local transmission loop to await.
	if l.stateSnapCtr == nil {
		return nil
	}

	// Follow published queue snapshots until Execute removes the operation.
	var current sobject.SharedObjectStateSnapshot
	for {
		// Read each newly published queue state once.
		next, err := l.stateSnapCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
		_, localOps, err := current.GetOpQueue(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(localOps, func(op *sobject.QueuedSOOperation) bool {
			return op.GetLocalId() == localID
		}) {
			return nil
		}
	}
}

// waitForRootSeqno waits for the accepted root to reach the published body
// snapshot. The host watch may advance before Execute publishes that snapshot.
func (l *LocalSOHost) waitForRootSeqno(ctx context.Context, minSeqno uint64) (uint64, error) {
	// Use the same container as body readers and retain its cancellation behavior.
	var current sobject.SharedObjectStateSnapshot
	for {
		// Wait for a published snapshot at or beyond the accepted root.
		next, err := l.stateSnapCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return 0, err
		}
		current = next
		root, err := current.GetRootState(ctx)
		if err != nil {
			return 0, err
		}
		if root.GetInnerSeqno() < minSeqno {
			continue
		}

		// Decode through the published snapshot so success also establishes readability.
		rootInner, err := current.GetRootInner(ctx)
		if err != nil {
			return 0, err
		}
		if rootInner == nil {
			return 0, errors.New("root inner state is nil")
		}
		return rootInner.GetSeqno(), nil
	}
}

// localOpResultOutcome resolves a persisted acceptance or rejection when available.
func (l *LocalSOHost) localOpResultOutcome(
	ctx context.Context,
	localID string,
) (uint64, bool, error, bool) {
	// Read the durable result for this caller's operation identifier.
	localOpResult, err := l.readLocalOpResult(ctx, localID)
	if err != nil {
		return 0, false, err, true
	}
	if localOpResult == nil || localOpResult.GetLocalId() != localID {
		return 0, false, nil, false
	}

	// Preserve the rejection's error details for the caller.
	if errorDetails := localOpResult.GetResult().GetErrorDetails(); errorDetails != nil {
		errorMsg := errorDetails.GetErrorMsg()
		if errorMsg != "" {
			return 0, true, errors.Wrap(sobject.ErrRejectedOp, errorMsg), true
		}
		return 0, true, sobject.ErrRejectedOp, true
	}

	// Legacy success records without a root sequence require host reconciliation.
	if seqno := localOpResult.GetRootSeqno(); seqno != 0 {
		return seqno, false, nil, true
	}
	return 0, false, nil, false
}

// readLocalState reads the local state from the object store.
func (l *LocalSOHost) readLocalState(ctx context.Context) (*LocalSOState, error) {
	// Read the local queue from one object-store snapshot.
	localStateKey := SobjectObjectStoreLocalStateKey(l.soHost.GetSharedObjectID())
	var lstate *LocalSOState
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return l.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Load the encoded queue, allowing a new SharedObject to start empty.
			localStateData, found, err := tx.Get(ctx, localStateKey)
			if err != nil {
				return err
			}

			// Decode the queue independently of the transaction's buffers.
			next := &LocalSOState{}
			if found {
				if err := next.UnmarshalVT(localStateData); err != nil {
					return err
				}
			}
			lstate = next
			return nil
		},
	)
	return lstate, err
}

// writeLocalState writes the local state to the object store.
func (l *LocalSOHost) writeLocalState(ctx context.Context, next *LocalSOState) error {
	// Trace persistence of one immutable local queue value.
	ctx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state")
	defer task.End()

	// Encode once so a transaction retry writes the identical queue.
	localStateKey := SobjectObjectStoreLocalStateKey(l.soHost.GetSharedObjectID())
	data, err := next.MarshalVT()
	if err != nil {
		return err
	}
	defer scrub.Scrub(data)

	// Write the prepared queue through the object store's transaction boundary.
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			taskCtx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/new-transaction")
			tx, err := l.objStore.NewTransaction(taskCtx, true)
			task.End()
			return tx, err
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			taskCtx, task := trace.NewTask(ctx, "alpha/local-so/write-local-state/set")
			err := tx.Set(taskCtx, localStateKey, data)
			task.End()
			return err
		},
	)
}

// readLocalOpResult reads the operation result from the object store.
func (l *LocalSOHost) readLocalOpResult(ctx context.Context, localOpID string) (*LocalSOOperationResult, error) {
	// Read the result stored under the caller's local operation identifier.
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), localOpID)
	var opResult *LocalSOOperationResult
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return l.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// An absent record leaves acceptance unresolved.
			opResultData, found, err := tx.Get(ctx, opResultKey)
			if err != nil {
				return err
			}
			if !found {
				opResult = nil
				return nil
			}

			// Decode the record and enforce its operation-key identity.
			next := &LocalSOOperationResult{}
			if err := next.UnmarshalVT(opResultData); err != nil {
				return err
			}
			if next.GetLocalId() != localOpID {
				return errors.New("read result from storage has wrong local id")
			}
			opResult = next
			return nil
		},
	)
	return opResult, err
}

// writeLocalOpResult durably writes the operation result to the object store.
func (l *LocalSOHost) writeLocalOpResult(ctx context.Context, result *LocalSOOperationResult) error {
	return l.putLocalOpResult(ctx, result, false)
}

// writeAcceptedLocalOpResult records an accepted operation's result with write
// ordering only. The accepted head that carries the operation is already
// durable, and the full commit that removed it from the local queue precedes
// this one, so a crash can lose only the record, never the operation or a
// replay of it.
func (l *LocalSOHost) writeAcceptedLocalOpResult(ctx context.Context, result *LocalSOOperationResult) error {
	return l.putLocalOpResult(ctx, result, true)
}

// putLocalOpResult writes the operation result, with write ordering only if
// ordered is set.
func (l *LocalSOHost) putLocalOpResult(ctx context.Context, result *LocalSOOperationResult, ordered bool) error {
	// Encode one result for identical writes across transaction retries.
	ctx, task := trace.NewTask(ctx, "alpha/local-so/write-local-op-result")
	defer task.End()
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), result.GetLocalId())
	data, err := result.MarshalVT()
	if err != nil {
		return err
	}
	defer scrub.Scrub(data)

	// Persist the acceptance or rejection atomically in the local object store.
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			tx, err := l.objStore.NewTransaction(ctx, true)
			if err != nil || !ordered {
				return tx, err
			}
			return kvtx.WithOrderedCommit(tx), nil
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, opResultKey, data)
		},
	)
}

// clearLocalOpResult clears the operation result from the object store.
func (l *LocalSOHost) clearLocalOpResult(ctx context.Context, localID string) error {
	opResultKey := SobjectObjectStoreLocalOpResultKey(l.soHost.GetSharedObjectID(), localID)
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return l.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Delete(ctx, opResultKey)
		},
	)
}

// decodeLocalRejectionError decrypts rejection details using the validator signer.
func (l *LocalSOHost) decodeLocalRejectionError(
	rejection *sobject.SOOperationRejection,
	inner *sobject.SOOperationRejectionInner,
) (*sobject.SOOperationRejectionErrorDetails, error) {
	// Derive the validator identity from the rejection's signature.
	validatorPublicKey, err := rejection.GetSignature().ParsePubKey()
	if err != nil {
		return nil, err
	}
	validatorPeerID, err := peer.IDFromPublicKey(validatorPublicKey)
	if err != nil {
		return nil, err
	}

	// Decrypt the rejection for this SharedObject and validator identity.
	return inner.DecodeErrorDetails(l.privKey, l.sharedObjectID, validatorPeerID)
}
