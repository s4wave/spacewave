package sobject

import (
	"bytes"
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

// Validate checks the configuration, root, grants, pending operations and
// rejections against the configuration's participants.
// Each submitter's rejections identify distinct operations.
func (s *SOState) Validate(sharedObjectID string) error {
	// The configuration and root must be structurally valid.
	if err := s.GetConfig().Validate(); err != nil {
		return errors.Wrap(err, "invalid config")
	}
	if err := s.GetRoot().Validate(); err != nil {
		return errors.Wrap(err, "invalid root")
	}
	participants := s.GetConfig().GetParticipants()
	roles := make(map[string]SOParticipantRole, len(participants))
	for _, participant := range participants {
		roles[participant.GetPeerId()] = participant.GetRole()
	}

	// Each grant is signed and belongs to one distinct reader.
	seenGrantPeerIDs := make(map[string]struct{}, len(s.GetRootGrants()))
	for i, grant := range s.GetRootGrants() {
		if err := grant.Validate(); err != nil {
			return errors.Wrapf(err, "root_grants[%d]", i)
		}
		peerID := grant.GetPeerId()
		if _, ok := seenGrantPeerIDs[peerID]; ok {
			return errors.Errorf("root_grants[%d]: duplicate peer id: %s", i, peerID)
		}
		seenGrantPeerIDs[peerID] = struct{}{}
		if err := grant.ValidateSignature(sharedObjectID, participants); err != nil {
			return errors.Wrapf(err, "root_grants[%d]", i)
		}
		if !CanReadState(roles[peerID]) {
			return errors.Errorf("peer %s has grant but no read access in participants", peerID)
		}
	}

	// Pending operations are signed and strictly increase in nonce per peer.
	seenOpNonces := make(map[string]uint64)
	queuedLocalIDs := make(map[string]map[string]struct{})
	queuedNonces := make(map[string]map[uint64]struct{})
	for i, op := range s.GetOps() {
		if err := op.Validate(); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}
		if err := op.ValidateSignature(sharedObjectID, participants); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}
		inner := &SOOperationInner{}
		if err := inner.UnmarshalVT(op.GetInner()); err != nil {
			return errors.Wrapf(err, "ops[%d]: failed to unmarshal inner data", i)
		}
		if err := inner.Validate(); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}
		peerID, nonce := inner.GetPeerId(), inner.GetNonce()
		if lastNonce, ok := seenOpNonces[peerID]; ok && nonce <= lastNonce {
			return errors.Errorf("ops[%d]: duplicate or out-of-order nonce for peer %s", i, peerID)
		}
		seenOpNonces[peerID] = nonce
		if queuedLocalIDs[peerID] == nil {
			queuedLocalIDs[peerID] = make(map[string]struct{})
			queuedNonces[peerID] = make(map[uint64]struct{})
		}
		if _, ok := queuedLocalIDs[peerID][inner.GetLocalId()]; ok {
			return errors.Errorf("ops[%d]: duplicate local id for peer %s", i, peerID)
		}
		queuedLocalIDs[peerID][inner.GetLocalId()] = struct{}{}
		queuedNonces[peerID][nonce] = struct{}{}
	}

	// Queued nonces hold one entry per peer, sorted by peer ID.
	if !slices.IsSortedFunc(s.GetQueuedAccountNonces(), func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	}) {
		return errors.New("queued account nonces not sorted by peer_id")
	}
	seenQueuedPeerIDs := make(map[string]struct{}, len(s.GetQueuedAccountNonces()))
	for i, nonce := range s.GetQueuedAccountNonces() {
		if err := nonce.Validate(); err != nil {
			return errors.Wrapf(err, "queued_account_nonces[%d]", i)
		}
		if _, ok := seenQueuedPeerIDs[nonce.GetPeerId()]; ok {
			return errors.Errorf("queued_account_nonces[%d]: duplicate peer id", i)
		}
		seenQueuedPeerIDs[nonce.GetPeerId()] = struct{}{}
	}

	// Rejections form one group per submitter, sorted by peer ID.
	if !slices.IsSortedFunc(s.GetOpRejections(), func(a, b *SOPeerOpRejections) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	}) {
		return errors.New("op_rejections is not sorted by peer_id")
	}
	seenRejectionPeerIDs := make(map[string]struct{}, len(s.GetOpRejections()))
	for i, peerRejections := range s.GetOpRejections() {
		if err := peerRejections.Validate(); err != nil {
			return errors.Wrapf(err, "op_rejections[%d]", i)
		}
		peerID := peerRejections.GetPeerId()
		if _, ok := seenRejectionPeerIDs[peerID]; ok {
			return errors.Errorf("op_rejections[%d]: duplicate peer id: %s", i, peerID)
		}
		seenRejectionPeerIDs[peerID] = struct{}{}

		// Each signed rejection names this submitter and one distinct operation.
		seenNonces := make(map[uint64]struct{}, len(peerRejections.GetRejections()))
		seenLocalIDs := make(map[string]struct{}, len(peerRejections.GetRejections()))
		for j, rejection := range peerRejections.GetRejections() {
			inner, err := rejection.ValidateSignature(sharedObjectID, participants)
			if err != nil {
				return errors.Wrapf(err, "op_rejections[%d].rejections[%d]", i, j)
			}
			if inner.GetPeerId() != peerID {
				return errors.Errorf("op_rejections[%d].rejections[%d]: peer id %s does not match group", i, j, inner.GetPeerId())
			}
			if _, ok := seenNonces[inner.GetOpNonce()]; ok {
				return errors.Errorf("op_rejections[%d].rejections[%d]: duplicate op nonce %d", i, j, inner.GetOpNonce())
			}
			seenNonces[inner.GetOpNonce()] = struct{}{}
			if _, ok := seenLocalIDs[inner.GetLocalId()]; ok {
				return errors.Errorf("op_rejections[%d].rejections[%d]: duplicate local id %s", i, j, inner.GetLocalId())
			}
			seenLocalIDs[inner.GetLocalId()] = struct{}{}
			if _, ok := queuedLocalIDs[peerID][inner.GetLocalId()]; ok {
				return errors.Errorf("op_rejections[%d].rejections[%d]: local id is still queued", i, j)
			}
			if _, ok := queuedNonces[peerID][inner.GetOpNonce()]; ok {
				return errors.Errorf("op_rejections[%d].rejections[%d]: nonce is still queued", i, j)
			}
		}
	}

	return nil
}

// UpdateRootState advances the held root by exactly one sequence number.
//
// The next root must carry consensus from the held configuration and, when
// enforceValidatorPeerID is non-empty, a signature from that validator.
// Rejections are recorded once per operation. Pending operations resolved by
// the root, acceptedOps or a rejection are removed. If an error is returned
// the SOState should be considered invalid.
func (s *SOState) UpdateRootState(
	sharedObjectID string,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
	rejectedOps []*SOOperationRejection,
	acceptedOps []*SOOperation,
) error {
	// Authorize the next root before changing held state.
	if err := s.validateNextRootState(sharedObjectID, nextRootState, enforceValidatorPeerID); err != nil {
		return err
	}

	// Parse the rejections and the accepted batch before applying either. The
	// accepted batch drains exactly even if account nonces lag in tests or
	// partial replay data.
	innerRejectedOps := make([]*SOOperationRejectionInner, len(rejectedOps))
	for i, ro := range rejectedOps {
		var err error
		innerRejectedOps[i], err = ro.UnmarshalInner()
		if err != nil {
			return err
		}
	}
	innerAcceptedOps := make([]*SOOperationInner, len(acceptedOps))
	for i, ao := range acceptedOps {
		var err error
		innerAcceptedOps[i], err = ao.UnmarshalInner()
		if err != nil {
			return err
		}
	}

	// Record new rejections before pruning resolved pending operations.
	for i, rejection := range rejectedOps {
		s.recordRejection(rejection, innerRejectedOps[i])
	}
	slices.SortFunc(s.OpRejections, func(a, b *SOPeerOpRejections) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})

	// Preserve consumed nonces even when an explicitly accepted batch is ahead
	// of the root's account nonce projection.
	for i, ao := range acceptedOps {
		inner := innerAcceptedOps[i]
		s.QueuedAccountNonces = advanceAccountNonce(s.QueuedAccountNonces, inner.GetPeerId(), inner.GetNonce(), ao.Hash())
	}

	// Advance the root and drop pending state it resolves.
	s.Root = nextRootState.CloneVT()
	s.Ops = FilterResolvedOperations(
		s.Ops,
		s.Root.GetAccountNonces(),
		innerAcceptedOps,
		s.GetOpRejections(),
	)
	s.updateQueuedNonces()

	return s.Validate(sharedObjectID)
}

// validateNextRootState checks that the next root follows the held root and
// carries consensus from the held configuration's validators.
func (s *SOState) validateNextRootState(
	sharedObjectID string,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
) error {
	// The root advances by exactly one sequence number.
	if nextRootState.GetInnerSeqno() != s.GetRoot().GetInnerSeqno()+1 {
		return ErrInvalidSeqno
	}

	// Every signature is valid, distinct and from a validator, and together
	// they satisfy the configured consensus mode.
	if err := nextRootState.Validate(); err != nil {
		return err
	}

	// A later root cannot forget operations already committed by an earlier one.
	nextNonces := make(map[string]uint64, len(nextRootState.GetAccountNonces()))
	for _, nonce := range nextRootState.GetAccountNonces() {
		nextNonces[nonce.GetPeerId()] = nonce.GetNonce()
	}
	for _, nonce := range s.GetRoot().GetAccountNonces() {
		if nextNonces[nonce.GetPeerId()] < nonce.GetNonce() {
			return errors.Wrap(ErrInvalidNonce, "root account nonce rollback")
		}
	}

	// The held configuration authorizes every signature on the next root.
	validSigs, err := nextRootState.ValidateSignatures(
		sharedObjectID,
		s.GetConfig().GetParticipants(),
	)
	if err != nil {
		return err
	}
	if err := CheckConsensusAcceptance(s.GetConfig().GetConsensusMode(), validSigs); err != nil {
		return err
	}

	// A caller that knows the expected validator requires its signature.
	if enforceValidatorPeerID == "" {
		return nil
	}
	return s.validateEnforcedValidator(nextRootState, enforceValidatorPeerID)
}

// validateEnforcedValidator checks that the enforced validator signed the root.
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

// updateQueuedNonces drops queued nonces the root has committed.
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

// recordRejection records a rejection under its submitter once.
// A replayed rejection of an already rejected operation is ignored.
// The caller restores the peer ID order of OpRejections.
func (s *SOState) recordRejection(rejection *SOOperationRejection, inner *SOOperationRejectionInner) {
	// Append to the submitter's group unless the operation is already rejected.
	peerID := inner.GetPeerId()
	for _, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() != peerID {
			continue
		}
		for _, existing := range peerRejections.GetRejections() {
			existingInner, err := existing.UnmarshalInner()
			if err == nil &&
				existingInner.GetOpNonce() == inner.GetOpNonce() &&
				existingInner.GetLocalId() == inner.GetLocalId() {
				return
			}
		}
		peerRejections.Rejections = append(peerRejections.Rejections, rejection)
		return
	}

	// Start a group for a submitter with no prior rejections.
	s.OpRejections = append(s.OpRejections, &SOPeerOpRejections{
		PeerId:     peerID,
		Rejections: []*SOOperationRejection{rejection},
	})
}

// GetOperationStatus returns the pending operation or the rejection with the
// given submitter and local ID. It returns nil, nil, nil if neither exists.
func (s *SOState) GetOperationStatus(peerID, localID string) (*SOOperation, *SOOperationRejection, error) {
	// A pending operation takes precedence over a rejection.
	for _, op := range s.Ops {
		inner, err := op.UnmarshalInner()
		if err != nil {
			return nil, nil, err
		}
		if inner.GetPeerId() == peerID && inner.GetLocalId() == localID {
			return op, nil, nil
		}
	}

	// Rejections are grouped by submitter.
	for _, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() != peerID {
			continue
		}
		for _, rejection := range peerRejections.GetRejections() {
			rejInner, err := rejection.UnmarshalInner()
			if err != nil {
				return nil, nil, err
			}
			if rejInner.GetLocalId() == localID {
				return nil, rejection, nil
			}
		}
		break
	}

	return nil, nil, nil
}

// NextOperationLink returns where peerID's next operation goes: one past the
// head of its chain, with every other participant's head as a parent, under
// the current config. The head is the latest operation that is pending,
// queued, or resolved by the root, applied or rejected.
func (s *SOState) NextOperationLink(peerID string) *SOOperationLink {
	// Extend the author's own head.
	heads := s.authorHeads()
	link := &SOOperationLink{ConfigHash: s.GetConfig().GetConfigChainHash()}
	if head, ok := heads[peerID]; ok {
		link.Nonce = head.GetNonce() + 1
		link.PrevOpHash = head.GetOpHash()
	} else {
		link.Nonce = 1
	}

	// Name the head of every other current participant.
	for _, p := range s.GetConfig().GetParticipants() {
		if head, ok := heads[p.GetPeerId()]; ok && p.GetPeerId() != peerID {
			link.ParentHashes = append(link.ParentHashes, head.GetOpHash())
		}
	}
	return link
}

// authorHeads returns the latest known operation of each author.
func (s *SOState) authorHeads() map[string]*SOAccountNonce {
	// Keep the highest sequence seen for each author.
	heads := make(map[string]*SOAccountNonce)
	advance := func(peerID string, nonce uint64, opHash []byte) {
		if head, ok := heads[peerID]; !ok || nonce > head.GetNonce() {
			heads[peerID] = &SOAccountNonce{PeerId: peerID, Nonce: nonce, OpHash: opHash}
		}
	}

	// Resolved, queued, and pending operations all advance a head.
	for _, nonce := range s.GetRoot().GetAccountNonces() {
		advance(nonce.GetPeerId(), nonce.GetNonce(), nonce.GetOpHash())
	}
	for _, nonce := range s.GetQueuedAccountNonces() {
		advance(nonce.GetPeerId(), nonce.GetNonce(), nonce.GetOpHash())
	}
	for _, op := range s.GetOps() {
		inner, err := op.UnmarshalInner()
		if err == nil {
			advance(inner.GetPeerId(), inner.GetNonce(), op.Hash())
		}
	}
	return heads
}

// QueueOperation queues a signed operation from a writer or validator.
// The operation must extend the head of its author's chain and carry a local
// ID that is neither pending nor rejected.
func (s *SOState) QueueOperation(sharedObjectID string, op *SOOperation) error {
	// Admit only a signed, next-in-sequence, unique operation.
	inner, err := s.validateOperation(sharedObjectID, op)
	if err != nil {
		return err
	}
	if err := s.validateOperationChain(inner); err != nil {
		return err
	}
	if err := s.validateOperationUnique(inner); err != nil {
		return err
	}

	// Advance the author's head and queue the operation.
	s.QueuedAccountNonces = advanceAccountNonce(s.QueuedAccountNonces, inner.GetPeerId(), inner.GetNonce(), op.Hash())
	s.Ops = append(s.Ops, op)
	return nil
}

// validateOperation checks queue capacity, operation format and signature,
// and returns the parsed inner operation.
func (s *SOState) validateOperation(
	sharedObjectID string,
	op *SOOperation,
) (*SOOperationInner, error) {
	if len(s.Ops) >= MaxOperations {
		return nil, errors.Wrap(ErrMaxCountExceeded, "operation queue")
	}
	if err := op.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid operation")
	}
	if err := op.ValidateSignature(sharedObjectID, s.GetConfig().GetParticipants()); err != nil {
		return nil, errors.Wrap(err, "failed to verify operation signature")
	}
	inner, err := op.UnmarshalInner()
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal operation inner data")
	}
	return inner, nil
}

// validateOperationChain checks that the operation directly follows the head
// of its author's chain.
func (s *SOState) validateOperationChain(inner *SOOperationInner) error {
	next := s.NextOperationLink(inner.GetPeerId())
	if inner.GetNonce() != next.Nonce {
		return errors.Wrapf(ErrInvalidNonce, "expected %d, got %d", next.Nonce, inner.GetNonce())
	}
	if !bytes.Equal(inner.GetPrevOpHash(), next.PrevOpHash) {
		return errors.Wrap(ErrInvalidNonce, "operation does not follow its author's previous operation")
	}
	return nil
}

// validateOperationUnique ensures the operation is not already queued or rejected.
func (s *SOState) validateOperationUnique(inner *SOOperationInner) error {
	peerID, localID := inner.GetPeerId(), inner.GetLocalId()
	existingOp, existingReject, err := s.GetOperationStatus(peerID, localID)
	if err != nil {
		return err
	}
	if existingOp != nil {
		return errors.Errorf("operation with localID %s already exists for peer %s", localID, peerID)
	}
	if existingReject != nil {
		return errors.Errorf("rejection with localID %s already exists for peer %s", localID, peerID)
	}
	return nil
}

// ClearOperationResult removes a rejection at the request of the peer that
// submitted the rejected operation. Clearing an absent rejection succeeds.
func (s *SOState) ClearOperationResult(sharedObjectID string, clearOp *SOClearOperationResult) error {
	// Parse the request and bind its inner peer ID to the signing key.
	if err := clearOp.Validate(); err != nil {
		return err
	}
	signerPubKey, err := clearOp.GetSignature().ParsePubKey()
	if err != nil {
		return err
	}
	signerPeerID, err := peer.IDFromPublicKey(signerPubKey)
	if err != nil {
		return err
	}
	signerPeerIDStr := signerPeerID.String()
	inner := &SOClearOperationResultInner{}
	if err := inner.UnmarshalVT(clearOp.GetInner()); err != nil {
		return errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return errors.Wrap(err, "invalid inner data")
	}
	if inner.GetPeerId() != signerPeerIDStr {
		return errors.New("signer peer ID does not match inner peer ID")
	}

	// Verify the signature in this shared object's context.
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

	// Remove the signer's rejection with that local ID, and its group if emptied.
	for i, peerRejections := range s.OpRejections {
		if peerRejections.GetPeerId() != signerPeerIDStr {
			continue
		}
		for j, rejection := range peerRejections.GetRejections() {
			rejInner, err := rejection.UnmarshalInner()
			if err != nil {
				return err
			}
			if rejInner.GetLocalId() != inner.GetLocalId() {
				continue
			}

			peerRejections.Rejections = slices.Delete(peerRejections.Rejections, j, j+1)
			if len(peerRejections.GetRejections()) == 0 {
				s.OpRejections = slices.Delete(s.OpRejections, i, i+1)
			}
			return nil
		}
		return nil
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*SOState)(nil)
