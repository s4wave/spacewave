package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// SOStateWatchFunc is a function to watch the SOState for changes.
type SOStateWatchFunc = func(ctx context.Context, sharedObjectID string, released func()) (ccontainer.Watchable[*SOState], func(), error)

// SOStateLockFunc is a function to lock and load the SOState.
type SOStateLockFunc = func(ctx context.Context, sharedObjectID string) (SOStateLock, error)

// SOHost is the implementation of the shared object host logic for a SOState container.
type SOHost struct {
	// watchFn is the function to call to watch the SOState.
	watchFn SOStateWatchFunc
	// lockFn is the function to call to lock and load the SOState.
	lockFn SOStateLockFunc
	// syncFuncs supplies provider-specific peer import and history reads.
	syncFuncs SOHostSyncFuncs
	// sharedObjectID is the id of the shared object.
	sharedObjectID string
	// soRc contains the shared object refcount instance.
	soRc *refcount.RefCount[ccontainer.Watchable[*SOState]]
}

// NewSOHost constructs a new shared object host.
//
// ctx can be nil.
func NewSOHost(ctx context.Context, watchFn SOStateWatchFunc, lockFn SOStateLockFunc, sharedObjectID string, syncFuncs ...*SOHostSyncFuncs) *SOHost {
	h := &SOHost{watchFn: watchFn, lockFn: lockFn, sharedObjectID: sharedObjectID}
	if len(syncFuncs) != 0 && syncFuncs[0] != nil {
		h.syncFuncs = *syncFuncs[0]
	}
	h.soRc = refcount.NewRefCount(ctx, false, nil, nil, func(ctx context.Context, released func()) (ccontainer.Watchable[*SOState], func(), error) {
		stateCtr, relStateCtr, err := watchFn(ctx, sharedObjectID, released)
		return stateCtr, relStateCtr, err
	})
	return h
}

// SetContext updates the context on the refcount.
//
// Returns if the context was updated.
func (s *SOHost) SetContext(ctx context.Context) bool {
	return s.soRc.SetContext(ctx)
}

// ClearContext clears the context and shuts down all routines.
func (s *SOHost) ClearContext() {
	s.soRc.ClearContext()
}

// GetSharedObjectID returns the sharedObjectID for the SOHost.
func (s *SOHost) GetSharedObjectID() string {
	return s.sharedObjectID
}

// CanWatchSOState returns true if the host can watch shared object state.
func (s *SOHost) CanWatchSOState() bool {
	return s.watchFn != nil
}

// GetSOStateCtr watches the shared object state with the refcount container.
func (s *SOHost) GetSOStateCtr(ctx context.Context, released func()) (ccontainer.Watchable[*SOState], func(), error) {
	return s.soRc.ResolveWithReleased(ctx, released)
}

// GetHostState returns a snapshot of the current SOState.
func (s *SOHost) GetHostState(ctx context.Context) (*SOState, error) {
	// Retain the watched state until its snapshot has been copied.
	watchable, rel, err := s.soRc.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	// Return an independent snapshot once the state becomes available.
	st, err := watchable.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return st.CloneVT(), nil
}

// GetRootState returns a snapshot of the current root state.
func (s *SOHost) GetRootState(ctx context.Context) (*SORoot, error) {
	hs, err := s.GetHostState(ctx)
	if err != nil {
		return nil, err
	}

	return hs.GetRoot(), nil
}

// GetRootInnerState returns a snapshot of the SORoot and unmarshals the SORootInner.
func (s *SOHost) GetRootInnerState(ctx context.Context) (*SORootInner, *SORoot, error) {
	// Read the signed root from the current host snapshot.
	sr, err := s.GetRootState(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Decode and validate the signed root's inner value.
	sri := &SORootInner{}
	if err := sri.UnmarshalVT(sr.GetInner()); err != nil {
		return nil, sr, err
	}
	return sri, sr, sri.Validate()
}

// UpdateSOState locks the SO state, clones it, calls the provided function
// to mutate the clone, then writes the updated state.
func (s *SOHost) UpdateSOState(ctx context.Context, fn func(state *SOState) error) error {
	// Hold the provider lock through mutation and persistence.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Apply the mutation to an independent state and commit on success.
	nextState := lk.GetSOState().CloneVT()
	if err := fn(nextState); err != nil {
		return err
	}
	return lk.WriteSOState(ctx, nextState)
}

// WaitDurable waits until every state write completed before the call is
// durable. Callers that send state off the machine capture it, call
// WaitDurable, then send, so no peer sees a state a power loss could undo.
func (s *SOHost) WaitDurable(ctx context.Context) error {
	if s.syncFuncs.WaitDurable == nil {
		return nil
	}
	return s.syncFuncs.WaitDurable(ctx)
}

// ReadConfigHistory returns retained transitions between exact configuration heads.
func (s *SOHost) ReadConfigHistory(ctx context.Context, base, target []byte) ([]*SOConfigChange, error) {
	if bytes.Equal(base, target) && len(base) != 0 {
		return nil, nil
	}
	if s.syncFuncs.History == nil {
		return nil, ErrConfigHistoryUnavailable
	}
	return s.syncFuncs.History(ctx, s.sharedObjectID, base, target)
}

// unappliedConfigChanges drops the proof prefix through the held checkpoint's
// head when the proof does not already link to it. Ordinary sync can apply part
// of a peer's proof while it is in flight; the remaining changes must still link
// to that head.
func unappliedConfigChanges(head []byte, changes []*SOConfigChange) ([]*SOConfigChange, error) {
	if len(changes) == 0 || bytes.Equal(changes[0].GetPreviousHash(), head) {
		return changes, nil
	}
	for i, change := range changes {
		hash, err := HashSOConfigChange(change)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(hash, head) {
			return changes[i+1:], nil
		}
	}
	return changes, nil
}

// ReadConfigEntry returns the retained transition that produced an exact head.
func (s *SOHost) ReadConfigEntry(ctx context.Context, head []byte) (*SOConfigChange, error) {
	if s.syncFuncs.Entry == nil || len(head) == 0 {
		return nil, ErrConfigHistoryUnavailable
	}
	entry, err := s.syncFuncs.Entry(ctx, s.sharedObjectID, head)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrConfigHistoryUnavailable
	}
	hash, err := HashSOConfigChange(entry)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(hash, head) {
		return nil, ErrConfigHistoryUnavailable
	}
	return entry, nil
}

// ImportPeerSnapshot verifies a candidate against held authority and commits it
// with its lineage. Access validation runs under the provider lock and must not
// reacquire host state. A committed local removal returns ErrParticipantRevoked.
func (s *SOHost) ImportPeerSnapshot(
	ctx context.Context,
	candidate *SOState,
	changes []*SOConfigChange,
	localPeer peer.ID,
	validateAccess func(context.Context, *SOState) error,
) error {
	// Bound untrusted work before acquiring the provider's write lock.
	if candidate.SizeVT() > 10*1024*1024 || len(changes) > MaxConfigSuffixEntries {
		return ErrConfigHistoryUnavailable
	}
	var historyBytes int
	for _, change := range changes {
		historyBytes += change.SizeVT()
		if historyBytes > MaxConfigSuffixBytes {
			return ErrConfigHistoryUnavailable
		}
	}

	// Acquire the provider's import boundary, which never publishes remote roots.
	lockFn := s.syncFuncs.Lock
	if lockFn == nil {
		lockFn = s.lockFn
	}
	lock, err := lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Authenticate the exact configuration using the checkpoint held by this lock.
	previous := lock.GetSOState()
	changes, err = unappliedConfigChanges(previous.GetConfig().GetConfigChainHash(), changes)
	if err != nil {
		return err
	}
	if err := VerifyConfigChainSuffix(previous.GetConfig(), candidate.GetConfig(), changes); err != nil {
		return errors.Wrap(err, "peer snapshot configuration authority")
	}
	readable := false
	for _, participant := range candidate.GetConfig().GetParticipants() {
		if participant.GetPeerId() == localPeer.String() && CanReadState(participant.GetRole()) {
			readable = true
			break
		}
	}

	// Apply proven revocation independently of root progress or access to new keys.
	if !readable {
		if len(changes) == 0 {
			return ErrParticipantRevoked
		}
		next := previous.CloneVT()
		next.Config = candidate.GetConfig().CloneVT()
		next.RootGrants = nil
		next.Ops = nil
		next.QueuedAccountNonces = nil
		next.OpRejections = nil
		if err := lock.WriteSOState(ctx, next, changes...); err != nil {
			return err
		}
		return ErrParticipantRevoked
	}

	// Authenticate root progress; unchanged accepted roots may carry role updates.
	next := candidate.CloneVT()
	seqno := next.GetRoot().GetInnerSeqno()
	previousSeqno := previous.GetRoot().GetInnerSeqno()
	if seqno < previousSeqno {
		return errors.New("peer snapshot root rollback")
	}
	if seqno == previousSeqno {
		acceptedRoot := previous.GetRoot()
		candidateRoot := next.GetRoot()
		if !bytes.Equal(candidateRoot.GetInner(), acceptedRoot.GetInner()) ||
			!slices.EqualFunc(candidateRoot.GetAccountNonces(), acceptedRoot.GetAccountNonces(), func(a, b *SOAccountNonce) bool { return a.EqualVT(b) }) {
			return errors.New("peer snapshot conflicts with accepted root")
		}
		// The held checkpoint already authenticated this exact content. Later
		// membership changes do not revoke authority over historical roots.
		next.Root = acceptedRoot.CloneVT()
	} else if len(next.GetRoot().GetInner()) != 0 {
		// Catch-up may skip roots, but cannot forget a committed account nonce.
		nextNonces := make(map[string]uint64, len(next.GetRoot().GetAccountNonces()))
		for _, nonce := range next.GetRoot().GetAccountNonces() {
			nextNonces[nonce.GetPeerId()] = nonce.GetNonce()
		}
		for _, nonce := range previous.GetRoot().GetAccountNonces() {
			if nextNonces[nonce.GetPeerId()] < nonce.GetNonce() {
				return errors.Wrap(ErrInvalidNonce, "peer snapshot root account nonce rollback")
			}
		}
		validSigs, err := next.GetRoot().ValidateSignatures(s.sharedObjectID, next.GetConfig().GetParticipants())
		if err != nil {
			return errors.Wrap(err, "peer snapshot root authority")
		}
		if err := CheckConsensusAcceptance(next.GetConfig().GetConsensusMode(), validSigs); err != nil {
			return errors.Wrap(err, "peer snapshot consensus")
		}
	}
	if err := next.Validate(s.sharedObjectID); err != nil {
		return errors.Wrap(err, "peer snapshot state")
	}
	if validateAccess != nil {
		if err := validateAccess(ctx, next); err != nil {
			return errors.Wrap(err, "inaccessible peer snapshot")
		}
	}

	// Invitation capabilities are locally administered, not authenticated by a root.
	next.Invites = previous.CloneVT().Invites
	operations := next.Ops
	next.Ops = nil
	next.QueuedAccountNonces = nil
	for _, operation := range operations {
		if err := next.QueueOperation(s.sharedObjectID, operation); err != nil {
			return errors.Wrap(err, "peer snapshot operation")
		}
	}

	// Preserve unresolved local operations that remain admissible under new authority.
	for _, operation := range previous.GetOps() {
		inner, err := operation.UnmarshalInner()
		if err != nil {
			return err
		}
		existing, rejection, err := next.GetOperationStatus(inner.GetPeerId(), inner.GetLocalId())
		if err != nil {
			return err
		}
		if existing != nil || rejection != nil {
			continue
		}
		// Committed, revoked or over-capacity local operations cannot be requeued.
		if err := next.QueueOperation(s.sharedObjectID, operation); err != nil {
			continue
		}
	}

	// Avoid publishing unchanged snapshots back through watches or provider writes.
	if next.EqualVT(previous) {
		return nil
	}
	return lock.WriteSOState(ctx, next, changes...)
}

// InstallInviteSnapshot installs an explicit, externally authenticated invitation result.
// The caller must have authenticated the invitation owner and its response before
// calling this operation. Ordinary peer synchronization must use ImportPeerSnapshot.
func (s *SOHost) InstallInviteSnapshot(ctx context.Context, candidate *SOState) error {
	// Validate the new checkpoint before opening the provider's replacement boundary.
	if s.syncFuncs.CheckpointLock == nil || len(candidate.GetConfig().GetConfigChainHash()) == 0 {
		return ErrConfigHistoryUnavailable
	}
	if err := candidate.Validate(s.sharedObjectID); err != nil {
		return err
	}
	signatures, err := candidate.GetRoot().ValidateSignatures(s.sharedObjectID, candidate.GetConfig().GetParticipants())
	if err != nil {
		return err
	}
	if err := CheckConsensusAcceptance(candidate.GetConfig().GetConsensusMode(), signatures); err != nil {
		return err
	}

	// Commit state and its invitation-authenticated checkpoint under one provider lock.
	lock, err := s.syncFuncs.CheckpointLock(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lock.Release()
	return lock.WriteSOState(ctx, candidate.CloneVT())
}

// UpdateRootState locks the host state and applies the UpdateRootState operation.
//
// Admission failures leave the held state unchanged; persistence is atomic at the provider boundary.
// If enforceValidatorPeerID is non-empty, ensures the given validator is in the set of signatures.
func (s *SOHost) UpdateRootState(
	ctx context.Context,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
	rejectedOps []*SOOperationRejection,
	acceptedOps []*SOOperation,
) error {
	// Serialize root acceptance with other host mutations.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Clone the locked state before root validation.
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// Validate root authorization and the operation results together.
	err = nextState.UpdateRootState(s.sharedObjectID, nextRootState, enforceValidatorPeerID, rejectedOps, acceptedOps)
	if err != nil {
		return err
	}

	// Publish the accepted state through the provider's write boundary.
	return lk.WriteSOState(ctx, nextState)
}

// ClearRejectedOperation clears a rejected operation from the state.
// The clear operation must be signed by the peer that submitted the original operation.
func (s *SOHost) ClearRejectedOperation(ctx context.Context, clearOp *SOClearOperationResult) error {
	// Serialize rejection cleanup with other host mutations.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Clone the locked state before changing rejection records.
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// Validate the clearing signature against the original operation.
	if err := nextState.ClearOperationResult(s.sharedObjectID, clearOp); err != nil {
		return err
	}

	// Commit the accepted rejection cleanup.
	return lk.WriteSOState(ctx, nextState)
}

// ApplyConfigChange applies a signed SOConfigChange to the shared object state.
//
// Verifies chain integrity (previous_hash matches current config_chain_hash,
// authorization from the current config), then replaces the config with the
// entry's config and updates the config_chain_hash.
//
// If fn is non-nil it is called after the config is applied but before the
// state is written, allowing additional atomic mutations (e.g. grant issuance).
func (s *SOHost) ApplyConfigChange(ctx context.Context, entry *SOConfigChange, fn func(state *SOState) error) error {
	// Reject absent input before acquiring provider resources.
	if entry == nil {
		return errors.New("config change entry is nil")
	}
	entry = entry.CloneVT()

	// Hold the provider lock through verification and persistence.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Retain the prior state unchanged if verification or the callback fails.
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// Verify the transition against the configuration held under this lock.
	nextState.Config, err = VerifyConfigChange(nextState.GetConfig(), entry)
	if err != nil {
		return err
	}

	// Include associated state changes in the same provider write.
	if fn != nil {
		if err := fn(nextState); err != nil {
			return err
		}
	}

	return lk.WriteSOState(ctx, nextState, entry)
}

// QueuedOpsProcessor validates the pending operations of a locked state and
// returns the next root and operation results in the form UpdateRootState
// admits.
type QueuedOpsProcessor = func(
	ctx context.Context,
	state *SOState,
) (nextRoot *SORoot, rejectedOps []*SOOperationRejection, acceptedOps []*SOOperation, err error)

// QueueOperation locks the host state and applies the QueueOperation operation.
//
// Calls the callback to build the SOOperation with the given nonce.
//
// Returns an error if the operation cannot be queued or if the nonce doesn't match the expected value.
func (s *SOHost) QueueOperation(
	ctx context.Context,
	peerID peer.ID,
	cb func(nonce uint64) (*SOOperation, error),
) error {
	return s.QueueOperationAndProcess(ctx, peerID, cb, nil)
}

// QueueOperationAndProcess queues an operation like QueueOperation. When
// process is set, peerID must be a validator: the root that process returns
// for the queued state is admitted as by UpdateRootState and written with the
// queued operation in one state write. If processing or admission fails, only
// the queued operation is written, leaving it for the next validator pass.
func (s *SOHost) QueueOperationAndProcess(
	ctx context.Context,
	peerID peer.ID,
	cb func(nonce uint64) (*SOOperation, error),
	process QueuedOpsProcessor,
) error {
	// Serialize nonce selection and operation acceptance under the provider lock.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Clone the locked state before selecting an operation nonce.
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// Select the next nonce for this peer's queued operation.
	nextAccNonce := nextState.GetNextAccountNonce(peerID.String())

	// Build the operation with the selected nonce.
	op, err := cb(nextAccNonce)
	if err != nil {
		return err
	}

	// Validate and queue the operation in the cloned state.
	err = nextState.QueueOperation(s.sharedObjectID, op)
	if err != nil {
		return err
	}

	// Apply the validated root when processing succeeds.
	if process != nil {
		if validated, ok := s.processQueued(ctx, peerID, nextState, process); ok {
			nextState = validated
		}
	}

	// Commit the accepted operation.
	return lk.WriteSOState(ctx, nextState)
}

// processQueued runs process on a locked state and applies the root it
// returns to a copy. ok is false if processing or admission failed; the
// validator's next pass reports those errors.
func (s *SOHost) processQueued(
	ctx context.Context,
	validatorPeerID peer.ID,
	state *SOState,
	process QueuedOpsProcessor,
) (*SOState, bool) {
	nextRoot, rejectedOps, acceptedOps, err := process(ctx, state)
	if err != nil {
		return nil, false
	}
	validated := state.CloneVT()
	err = validated.UpdateRootState(s.sharedObjectID, nextRoot, validatorPeerID.String(), rejectedOps, acceptedOps)
	if err != nil {
		return nil, false
	}
	return validated, true
}
