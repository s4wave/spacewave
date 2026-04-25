package sobject

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// NewSOStateBlock constructs a new SOState block.
func NewSOStateBlock() block.Block {
	return &SOState{}
}

// UnmarshalSOState unmarshals a SOState from a block cursor.
func UnmarshalSOState(ctx context.Context, bcs *block.Cursor) (*SOState, error) {
	return block.UnmarshalBlock[*SOState](ctx, bcs, NewSOStateBlock)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *SOState) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *SOState) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate validates the SOState.
func (s *SOState) Validate(sharedObjectID string) error {
	if err := s.GetConfig().Validate(); err != nil {
		return errors.Wrap(err, "invalid config")
	}

	if err := s.GetRoot().Validate(); err != nil {
		return errors.Wrap(err, "invalid root")
	}

	// Validate root grants
	seenPeerIDs := make(map[string]struct{}, max(len(s.GetRootGrants()), len(s.GetOpRejections())))
	for i, grant := range s.GetRootGrants() {
		// validate basic properties
		if err := grant.Validate(); err != nil {
			return errors.Wrapf(err, "root_grants[%d]", i)
		}

		// validate peer id
		peerID := grant.GetPeerId()
		if _, ok := seenPeerIDs[peerID]; ok {
			return errors.Errorf("root_grants[%d]: duplicate peer id: %s", i, peerID)
		}
		seenPeerIDs[peerID] = struct{}{}

		// validate signature
		if err := grant.ValidateSignature(sharedObjectID, s.GetConfig().GetParticipants()); err != nil {
			return errors.Wrapf(err, "root_grants[%d]", i)
		}
	}

	// Validate that all participants that have grants have CanReadState role
	for peerID := range seenPeerIDs {
		var hasReadAccess bool
		for _, participant := range s.GetConfig().GetParticipants() {
			if participant.GetPeerId() == peerID {
				hasReadAccess = CanReadState(participant.GetRole())
				break
			}
		}
		if !hasReadAccess {
			return errors.Errorf("peer %s has grant but no read access in participants", peerID)
		}
	}

	// Validate operations
	seenOps := make(map[string]uint64)
	for i, op := range s.GetOps() {
		// validate basic properties
		if err := op.Validate(); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}

		// validate signature
		if err := op.ValidateSignature(sharedObjectID, s.GetConfig().GetParticipants()); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}

		// Unmarshal the inner data to get the peer ID and nonce
		inner := &SOOperationInner{}
		if err := inner.UnmarshalVT(op.GetInner()); err != nil {
			return errors.Wrapf(err, "ops[%d]: failed to unmarshal inner data", i)
		}

		// Validate
		if err := inner.Validate(); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}

		// Check for duplicate (peer_id, nonce) pairs
		peerID := inner.GetPeerId()
		nonce := inner.GetNonce()
		if lastNonce, ok := seenOps[peerID]; ok && nonce <= lastNonce {
			return errors.Errorf("ops[%d]: duplicate or out-of-order nonce for peer %s", i, peerID)
		}
		seenOps[peerID] = nonce
	}

	// Ensure op_rejections is sorted.
	if !slices.IsSortedFunc(s.GetOpRejections(), func(a, b *SOPeerOpRejections) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	}) {
		return errors.New("op_rejections is not sorted by peer_id")
	}

	// Validate queued_account_nonces are sorted
	if !slices.IsSortedFunc(s.GetQueuedAccountNonces(), func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	}) {
		return errors.New("queued account nonces not sorted by peer_id")
	}

	// Validate queued nonces are unique by peer_id
	seenQueuedPeerIDs := make(map[string]struct{})
	for i, nonce := range s.GetQueuedAccountNonces() {
		if nonce.GetPeerId() == "" {
			return errors.Wrapf(peer.ErrEmptyPeerID, "queued_account_nonces[%d].peer_id", i)
		}
		if _, ok := seenQueuedPeerIDs[nonce.GetPeerId()]; ok {
			return errors.Errorf("queued_account_nonces[%d]: duplicate peer id", i)
		}
		seenQueuedPeerIDs[nonce.GetPeerId()] = struct{}{}
	}

	// Validate op_rejections
	clear(seenPeerIDs)
	for i, peerRejections := range s.GetOpRejections() {
		if err := peerRejections.Validate(); err != nil {
			return errors.Wrapf(err, "op_rejections[%d]", i)
		}
		peerID, err := peerRejections.ParsePeerID()
		if err != nil {
			return errors.Wrapf(err, "op_rejections[%d]: invalid peer id", i)
		}
		peerIDStr := peerID.String()
		if _, ok := seenPeerIDs[peerIDStr]; ok {
			return errors.Errorf("op_rejections[%d]: duplicate peer id: %s", i, peerIDStr)
		}
		seenPeerIDs[peerIDStr] = struct{}{}

		for j, rejection := range peerRejections.GetRejections() {
			inner, err := rejection.ValidateSignature(sharedObjectID, s.GetConfig().GetParticipants())
			if err != nil {
				return errors.Wrapf(err, "op_rejections[%d].rejections[%d]", i, j)
			}
			inner.Reset()
		}
	}

	return nil
}

// UpdateRootState updates the root state of a SOState.
//
// Applies the changes to the passed SOState.
// If an error is returned the SOState should be considered invalid.
// If enforceValidatorPeerID is non-empty, ensures the given validator is in the set of signatures.
func (s *SOState) UpdateRootState(
	sharedObjectID string,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
	rejectedOps []*SOOperationRejection,
	acceptedOps []*SOOperation,
) error {
	// Validate the next root state and signatures
	if err := s.validateNextRootState(sharedObjectID, nextRootState, enforceValidatorPeerID); err != nil {
		return err
	}

	// Parse all rejected ops.
	innerRejectedOps := make([]*SOOperationRejectionInner, len(rejectedOps))
	for i, ro := range rejectedOps {
		var err error
		innerRejectedOps[i], err = ro.UnmarshalInner()
		if err != nil {
			return err
		}
	}

	// Parse all accepted ops so the current root update can drain the exact
	// accepted batch even if account nonces lag in tests or partial replay data.
	innerAcceptedOps := make([]*SOOperationInner, len(acceptedOps))
	for i, ao := range acceptedOps {
		var err error
		innerAcceptedOps[i], err = ao.UnmarshalInner()
		if err != nil {
			return err
		}
	}

	// Process any new rejections before pruning resolved pending ops.
	s.processOperationUpdates(rejectedOps, innerRejectedOps)

	// Update the state root
	s.Root = nextRootState.CloneVT()

	// Remove ops resolved by the new root or any known rejection.
	s.Ops = FilterResolvedOperations(
		s.Ops,
		s.Root.GetAccountNonces(),
		innerAcceptedOps,
		s.GetOpRejections(),
	)

	// Update queued nonces
	s.updateQueuedNonces()

	// Validate final state
	return s.Validate(sharedObjectID)
}

// validateNextRootState validates the next root state and its signatures.
func (s *SOState) validateNextRootState(
	sharedObjectID string,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
) error {
	// Check that the seqno incremented by one.
	if nextRootState.GetInnerSeqno() != s.GetRoot().GetInnerSeqno()+1 {
		return ErrInvalidSeqno
	}

	// Validate the updated root state.
	// Checks there is at least one signature.
	if err := nextRootState.Validate(); err != nil {
		return err
	}

	// Validate the signatures and that all signatures are validators.
	validSigs, err := nextRootState.ValidateSignatures(
		sharedObjectID,
		s.GetConfig().GetParticipants(),
	)
	if err != nil {
		return err
	}

	// Check consensus acceptance based on the configured mode.
	if err := CheckConsensusAcceptance(s.GetConfig().GetConsensusMode(), validSigs); err != nil {
		return err
	}

	// Check the enforceValidatorPeerID if set.
	if enforceValidatorPeerID != "" {
		if err := s.validateEnforcedValidator(nextRootState, enforceValidatorPeerID); err != nil {
			return err
		}
	}

	return nil
}

// validateEnforcedValidator checks if the enforced validator is present in signatures
func (s *SOState) validateEnforcedValidator(nextRootState *SORoot, enforceValidatorPeerID string) error {
	for _, sig := range nextRootState.GetValidatorSignatures() {
		sigPub, err := sig.ParsePubKey()
		if err != nil {
			return err
		}

		sigPeerID, err := peer.IDFromPublicKey(sigPub)
		if err != nil {
			return err
		}

		if sigPeerID.String() == enforceValidatorPeerID {
			return nil
		}
	}
	return ErrInvalidValidator
}

// processOperationUpdates records newly observed rejections.
func (s *SOState) processOperationUpdates(
	rejectedOps []*SOOperationRejection,
	innerRejectedOps []*SOOperationRejectionInner,
) {
	// Process rejections
	for i, rejection := range rejectedOps {
		s.processRejection(rejection, innerRejectedOps[i])
	}

	// Update OpRejections sort order
	if len(rejectedOps) != 0 {
		s.updateOpRejections()
	}
}

// updateQueuedNonces updates the queued nonces based on root nonces
func (s *SOState) updateQueuedNonces() {
	s.QueuedAccountNonces = slices.DeleteFunc(s.QueuedAccountNonces, func(qNonce *SOAccountNonce) bool {
		for _, rNonce := range s.Root.GetAccountNonces() {
			if qNonce.GetPeerId() == rNonce.GetPeerId() && qNonce.GetNonce() <= rNonce.GetNonce() {
				return true
			}
		}
		return false
	})
}

// processRejection handles a single rejection
func (s *SOState) processRejection(rejection *SOOperationRejection, inner *SOOperationRejectionInner) {
	peerID := inner.GetPeerId()
	for i, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() == peerID {
			s.OpRejections[i].Rejections = append(s.OpRejections[i].Rejections, rejection)
			return
		}
	}
	s.OpRejections = append(s.OpRejections, &SOPeerOpRejections{
		PeerId:     peerID,
		Rejections: []*SOOperationRejection{rejection},
	})
}

// updateOpRejections updates the OpRejections list
func (s *SOState) updateOpRejections() {
	// Remove empty rejection lists
	s.OpRejections = slices.DeleteFunc(s.OpRejections, func(o *SOPeerOpRejections) bool {
		return len(o.GetRejections()) == 0
	})

	// Sort OpRejections by peer_id
	slices.SortFunc(s.OpRejections, func(a, b *SOPeerOpRejections) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
}

// GetOperationStatus returns the status of the given operation.
//
// Returns the matching SOOperation if the operation is queued.
// Returns the matching SOOperationRejection if the operation is rejected.
// Returns nil, nil, nil if not found.
func (s *SOState) GetOperationStatus(peerID, localID string) (*SOOperation, *SOOperationRejection, error) {
	// Check if the operation is in the pending queue
	for _, op := range s.Ops {
		inner := &SOOperationInner{}
		if err := inner.UnmarshalVT(op.GetInner()); err != nil {
			return nil, nil, err
		}
		if inner.GetPeerId() == peerID && inner.GetLocalId() == localID {
			return op, nil, nil
		}
	}

	// Check if the operation is in the rejections list
	for _, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() != peerID {
			continue
		}

		for _, rejection := range peerRejections.GetRejections() {
			rejInner := &SOOperationRejectionInner{}
			if err := rejInner.UnmarshalVT(rejection.GetInner()); err != nil {
				return nil, nil, err
			}
			if rejInner.GetLocalId() == localID {
				return nil, rejection, nil
			}
		}
		break
	}

	// If the operation is not found in either list, return nil, nil, nil
	return nil, nil, nil
}

// GetNextAccountNonce determines the next nonce for an account.
func (s *SOState) GetNextAccountNonce(peerID string) uint64 {
	var currentNonce uint64

	// Check queued_account_nonces first
	for _, nonce := range s.GetQueuedAccountNonces() {
		if nonce.GetPeerId() == peerID {
			currentNonce = nonce.GetNonce()
			break
		}
	}

	// If no queued nonce, check the root
	if currentNonce == 0 {
		for _, nonce := range s.GetRoot().GetAccountNonces() {
			if nonce.GetPeerId() == peerID {
				currentNonce = nonce.GetNonce()
				break
			}
		}
	}

	return currentNonce + 1
}

// QueueOperation queues an operation for a writer or validator.
// Returns an error if the operation cannot be queued or if the nonce doesn't match the expected value.
func (s *SOState) QueueOperation(sharedObjectID string, op *SOOperation) error {
	// Validate operation format and signature
	inner, err := s.validateOperation(sharedObjectID, op)
	if err != nil {
		return err
	}

	// Validate operation nonce
	if err := s.validateOperationNonce(inner); err != nil {
		return err
	}

	// Validate operation is unique
	if err := s.validateOperationUnique(inner); err != nil {
		return err
	}

	// Update the queued account nonce
	s.updateQueuedAccountNonce(inner.GetPeerId(), inner.GetNonce())

	// Add the operation to the queue
	s.Ops = append(s.Ops, op)

	return nil
}

// validateOperation validates an operation before queueing.
// Returns the parsed inner operation if valid.
func (s *SOState) validateOperation(
	sharedObjectID string,
	op *SOOperation,
) (*SOOperationInner, error) {
	// Check if we've reached the maximum number of operations
	if len(s.Ops) >= MaxOperations {
		return nil, errors.Wrap(ErrMaxCountExceeded, "operation queue")
	}

	// Validate the operation
	if err := op.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid operation")
	}

	// Verify the operation signature
	if err := op.ValidateSignature(sharedObjectID, s.GetConfig().GetParticipants()); err != nil {
		return nil, errors.Wrap(err, "failed to verify operation signature")
	}

	// Unmarshal the inner operation
	inner := &SOOperationInner{}
	if err := inner.UnmarshalVT(op.GetInner()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal operation inner data")
	}

	return inner, nil
}

// validateOperationNonce validates the operation nonce.
func (s *SOState) validateOperationNonce(inner *SOOperationInner) error {
	// Get the expected nonce for the peer
	peerID := inner.GetPeerId()
	expectedNonce := s.GetNextAccountNonce(peerID)

	// Check if the provided nonce matches the expected nonce
	if inner.GetNonce() != expectedNonce {
		return errors.Wrapf(ErrInvalidNonce, "expected %d, got %d", expectedNonce, inner.GetNonce())
	}

	return nil
}

// validateOperationUnique ensures the operation is not already queued or rejected.
func (s *SOState) validateOperationUnique(inner *SOOperationInner) error {
	peerIDStr := inner.GetPeerId()
	localID := inner.GetLocalId()

	existingOp, existingReject, err := s.GetOperationStatus(peerIDStr, localID)
	if err != nil {
		return err
	}
	if existingOp != nil {
		return errors.Errorf("operation with localID %s already exists for peer %s", localID, peerIDStr)
	}
	if existingReject != nil {
		return errors.Errorf("rejection with localID %s already exists for peer %s", localID, peerIDStr)
	}

	return nil
}

// updateQueuedAccountNonce updates the queued account nonce for a peer.
func (s *SOState) updateQueuedAccountNonce(peerID string, nonce uint64) {
	// Find and update existing nonce
	for i, qNonce := range s.QueuedAccountNonces {
		if qNonce.GetPeerId() == peerID {
			if nonce > qNonce.GetNonce() {
				s.QueuedAccountNonces[i].Nonce = nonce
			}
			return
		}
	}

	// Add new nonce
	s.QueuedAccountNonces = append(s.QueuedAccountNonces, &SOAccountNonce{
		PeerId: peerID,
		Nonce:  nonce,
	})

	// Sort by peer_id
	slices.SortFunc(s.QueuedAccountNonces, func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
}

// ClearOperationResult clears a rejected operation result from the state.
// Verifies the clear operation result signature and that the operation being cleared
// was submitted by the same peer that signed the clear request.
func (s *SOState) ClearOperationResult(sharedObjectID string, clearOp *SOClearOperationResult) error {
	// Validate the clear operation format
	if err := clearOp.Validate(); err != nil {
		return err
	}

	// Parse and validate the signature
	signerPubKey, err := clearOp.GetSignature().ParsePubKey()
	if err != nil {
		return err
	}
	signerPeerID, err := peer.IDFromPublicKey(signerPubKey)
	if err != nil {
		return err
	}
	signerPeerIDStr := signerPeerID.String()

	// Unmarshal the inner data
	inner := &SOClearOperationResultInner{}
	if err := inner.UnmarshalVT(clearOp.GetInner()); err != nil {
		return errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return errors.Wrap(err, "invalid inner data")
	}

	// Verify the peer IDs match
	if inner.GetPeerId() != signerPeerIDStr {
		return errors.New("signer peer ID does not match inner peer ID")
	}

	// Verify the signature
	encContext := BuildSOClearOperationResultSignatureContext(
		sharedObjectID,
		signerPeerIDStr,
		inner.GetLocalId(),
	)
	valid, err := clearOp.GetSignature().VerifyWithPublic(encContext, signerPubKey, clearOp.GetInner())
	if err != nil {
		return errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return peer.ErrSignatureInvalid
	}

	// Find and remove the rejection
	var found bool
	for i, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() != signerPeerIDStr {
			continue
		}

		// Look for the rejection with matching local ID
		rejections := peerRejections.GetRejections()
		nextRejections := make([]*SOOperationRejection, 0, len(rejections))
		for _, rejection := range rejections {
			rejInner, err := rejection.UnmarshalInner()
			if err != nil {
				return err
			}
			if rejInner.GetLocalId() != inner.GetLocalId() {
				nextRejections = append(nextRejections, rejection)
			} else {
				found = true
			}
		}

		// Update or remove the peer rejections entry
		if found {
			if len(nextRejections) == 0 {
				// Remove the peer rejections entry if empty
				s.OpRejections = append(s.OpRejections[:i], s.OpRejections[i+1:]...)
			} else {
				// Update the rejections list
				s.OpRejections[i].Rejections = nextRejections
			}
			break
		}
	}

	return nil
}

// _ is a type assertion
var _ block.Block = ((*SOState)(nil))
