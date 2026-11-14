package sobject

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
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
	// verifier builds the operation sets of the states of the shared object.
	verifier *SOOperationVerifier
	// keyring decrypts the local peer's grants in the states of the shared object.
	keyring *SOGrantKeyring
	// soRc contains the shared object refcount instance.
	soRc *refcount.RefCount[ccontainer.Watchable[*SOState]]
}

// NewSOHost constructs a new shared object host.
//
// ctx can be nil.
func NewSOHost(ctx context.Context, watchFn SOStateWatchFunc, lockFn SOStateLockFunc, sharedObjectID string, syncFuncs ...*SOHostSyncFuncs) *SOHost {
	h := &SOHost{watchFn: watchFn, lockFn: lockFn, sharedObjectID: sharedObjectID, verifier: NewSOOperationVerifier(sharedObjectID), keyring: NewSOGrantKeyring(sharedObjectID)}
	if len(syncFuncs) != 0 && syncFuncs[0] != nil {
		h.syncFuncs = *syncFuncs[0]
	}
	h.soRc = refcount.NewRefCount(ctx, false, nil, nil, func(ctx context.Context, released func()) (ccontainer.Watchable[*SOState], func(), error) {
		return watchFn(ctx, sharedObjectID, released)
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

// GetOperationVerifier returns the verifier of the shared object's operations.
// Snapshots of the host's states share it, so a state that gains an operation
// verifies only that one.
func (s *SOHost) GetOperationVerifier() *SOOperationVerifier {
	return s.verifier
}

// GetGrantKeyring returns the keyring of the local peer's grants. Snapshots of
// the host's states share it, so a new state decrypts only a new grant.
func (s *SOHost) GetGrantKeyring() *SOGrantKeyring {
	return s.keyring
}

// GetSOStateCtr watches the shared object state with the refcount container.
func (s *SOHost) GetSOStateCtr(ctx context.Context, released func()) (ccontainer.Watchable[*SOState], func(), error) {
	return s.soRc.ResolveWithReleased(ctx, released)
}

// GetHostState returns a snapshot of the current SOState.
// Its signed operations are immutable and shared with the held state.
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
	return st.Clone(), nil
}

// UpdateSOState locks the SO state, clones it, calls the provided function
// to mutate the clone, then writes the updated state with changes, the config
// changes it commits. fn may edit the state's fields and operation slice, but
// not signed operations.
func (s *SOHost) UpdateSOState(ctx context.Context, fn func(state *SOState) error, changes ...*SOConfigChange) error {
	// Hold the provider lock through mutation and persistence.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Apply the mutation to an independent state and commit on success.
	nextState := lk.GetSOState().Clone()
	if err := fn(nextState); err != nil {
		return err
	}
	return lk.WriteSOState(ctx, nextState, changes...)
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
	// Read the entry the head names.
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

// ReadConfigLineage returns the retained changes that lead to the config head
// target, oldest first. See ReadConfigLineage.
func (s *SOHost) ReadConfigLineage(ctx context.Context, target []byte) ([]*SOConfigChange, error) {
	if s.syncFuncs.Entry == nil {
		return nil, nil
	}
	return ReadConfigLineage(ctx, target, s.ReadConfigEntry)
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
	// Acquire the provider's import boundary for the bounded candidate.
	lock, err := s.lockPeerImport(ctx, candidate, changes)
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
	if err := s.checkMergeFloor(ctx, previous, changes); err != nil {
		return err
	}
	if err := VerifyConfigChainSuffix(s.sharedObjectID, previous.GetConfig(), candidate.GetConfig(), changes); err != nil {
		return errors.Wrap(err, "peer snapshot configuration authority")
	}
	return s.commitPeerSnapshot(ctx, lock, candidate, changes, localPeer, validateAccess)
}

// ImportPeerConfig adopts the changes that follow the held config head, each
// authorized by the config before it, without adopting any other state. It
// serves a participant whose config predates the key of a peer that holds a
// newer one, so it accepts only changes that extend the held head, never a
// branch to merge. It returns the number of changes adopted, and zero when
// changes do not extend the held head. A committed local removal returns
// ErrParticipantRevoked.
func (s *SOHost) ImportPeerConfig(
	ctx context.Context,
	changes []*SOConfigChange,
	localPeer peer.ID,
	validateAccess func(context.Context, *SOState) error,
) (int, error) {
	// Acquire the provider's import boundary for the bounded changes.
	lock, err := s.lockPeerImport(ctx, &SOState{}, changes)
	if err != nil {
		return 0, err
	}
	defer lock.Release()

	// Keep only the changes after the held head, matched by hash before any
	// signature is checked.
	previous := lock.GetSOState()
	head := previous.GetConfig().GetConfigChainHash()
	changes, err = unappliedConfigChanges(head, changes)
	if err != nil {
		return 0, err
	}
	if len(head) == 0 || len(changes) == 0 || !bytes.Equal(changes[0].GetPreviousHash(), head) {
		return 0, nil
	}

	// Authenticate each change against the config before it.
	config := previous.GetConfig()
	for i, change := range changes {
		if !isPeerConfigChange(change.GetChangeType()) {
			return 0, errors.Errorf("peer config change[%d]: unsupported change %s", i, change.GetChangeType())
		}
		config, err = VerifyConfigChange(s.sharedObjectID, config, change)
		if err != nil {
			return 0, errors.Wrapf(err, "peer config change[%d]", i)
		}
	}

	// Commit the config alone, as a snapshot that carries nothing else.
	if err := s.commitPeerSnapshot(ctx, lock, &SOState{Config: config}, changes, localPeer, validateAccess); err != nil {
		return 0, err
	}
	return len(changes), nil
}

// lockPeerImport bounds untrusted work, then acquires the provider's import
// boundary, which never publishes remote roots.
func (s *SOHost) lockPeerImport(ctx context.Context, candidate *SOState, changes []*SOConfigChange) (SOStateLock, error) {
	// Bound untrusted work before acquiring the provider's write lock.
	if candidate.SizeVT() > MaxSyncStateSize || len(changes) > MaxConfigSuffixEntries {
		return nil, ErrConfigHistoryUnavailable
	}
	var historyBytes int
	for _, change := range changes {
		historyBytes += change.SizeVT()
		if historyBytes > MaxConfigSuffixBytes {
			return nil, ErrConfigHistoryUnavailable
		}
	}

	// Acquire the lock.
	lockFn := s.syncFuncs.Lock
	if lockFn == nil {
		lockFn = s.lockFn
	}
	return lockFn(ctx, s.sharedObjectID)
}

// commitPeerSnapshot merges a candidate whose config changes, the suffix from
// the held config, the caller verified, and writes it under lock.
func (s *SOHost) commitPeerSnapshot(
	ctx context.Context,
	lock SOStateLock,
	candidate *SOState,
	changes []*SOConfigChange,
	localPeer peer.ID,
	validateAccess func(context.Context, *SOState) error,
) error {
	// Find whether the local peer can still read the candidate.
	previous := lock.GetSOState()
	readable := false
	for _, participant := range candidate.GetConfig().GetParticipants() {
		if participant.GetPeerId() == localPeer.String() && CanReadState(participant.GetRole()) {
			readable = true
			break
		}
	}

	// Apply proven revocation independently of access to new keys.
	if !readable {
		if len(changes) == 0 {
			return ErrParticipantRevoked
		}
		next := previous.Clone()
		next.Config = candidate.GetConfig().CloneVT()
		next.KeyEpochs = nil
		next.Ops = nil
		next.ControlMessages = nil
		if err := lock.WriteSOState(ctx, next, changes...); err != nil {
			return err
		}
		return ErrParticipantRevoked
	}

	// Merge the candidate into the held state. Invitations stay locally
	// administered.
	next := previous.Clone()
	if err := next.mergeSnapshot(s.sharedObjectID, candidate); err != nil {
		return err
	}

	// Check the merged state.
	if err := next.Validate(s.sharedObjectID); err != nil {
		return errors.Wrap(err, "peer snapshot state")
	}
	if validateAccess != nil {
		if err := validateAccess(ctx, next); err != nil {
			return errors.Wrap(err, "inaccessible peer snapshot")
		}
	}

	// Avoid publishing unchanged snapshots back through watches or provider writes.
	if next.EqualVT(previous) {
		return nil
	}
	return lock.WriteSOState(ctx, next, changes...)
}

// mergeSnapshot adopts the config and checkpoint of a candidate whose config
// the caller authenticated, then merges its grants, operations, sequence and
// control messages. The state keeps its own operations and invitations.
func (s *SOState) mergeSnapshot(sharedObjectID string, candidate *SOState) error {
	// Adopt the verified config and the candidate's checkpoint.
	s.Config = candidate.GetConfig().CloneVT()
	s.pruneControlMessages()
	if checkpoint := candidate.GetCheckpoint(); checkpoint != nil {
		if err := s.AdoptCheckpoint(sharedObjectID, checkpoint); err != nil {
			return errors.Wrap(err, "peer snapshot checkpoint")
		}
	}

	// Merge the grants, operations, sequence and control messages.
	if err := s.MergeKeyEpochs(sharedObjectID, candidate.GetKeyEpochs()); err != nil {
		return errors.Wrap(err, "peer snapshot key epochs")
	}
	for _, op := range candidate.GetOps() {
		if _, err := s.AddOperation(sharedObjectID, op); err != nil {
			return errors.Wrap(err, "peer snapshot operation")
		}
	}
	for _, record := range candidate.GetSequence() {
		if _, err := s.AddSequence(sharedObjectID, record); err != nil {
			return errors.Wrap(err, "peer snapshot sequence")
		}
	}
	s.MergeControlMessages(sharedObjectID, candidate.GetControlMessages())
	return nil
}

// InstallInviteSnapshot installs an explicit, externally authenticated
// invitation result with lineage, the inviter's retained changes leading to
// its config, oldest first. The lineage lets this replica resolve the config
// of every operation the candidate holds. The caller must have authenticated
// the invitation owner and its response before calling this operation.
//
// The invitation replaces the held state, so the caller must hold no state it
// needs: a replica that holds the object calls MergeInviteSnapshot instead.
// Ordinary peer synchronization must use ImportPeerSnapshot.
func (s *SOHost) InstallInviteSnapshot(ctx context.Context, candidate *SOState, lineage []*SOConfigChange) error {
	return s.writeInviteSnapshot(ctx, candidate, lineage, func(*SOState) (*SOState, error) {
		return candidate.Clone(), nil
	})
}

// MergeInviteSnapshot merges an invitation result into the state this replica
// holds. The lineage must extend the held config, which authorizes every
// change, so the replica keeps its own operations and invitations. A stale
// invitation, one from an unrelated chain, or one beyond the retained lineage
// is refused, because installing it would roll back or replace the authority
// the replica holds. See InstallInviteSnapshot for the arguments.
func (s *SOHost) MergeInviteSnapshot(ctx context.Context, candidate *SOState, lineage []*SOConfigChange) error {
	return s.writeInviteSnapshot(ctx, candidate, lineage, func(held *SOState) (*SOState, error) {
		return s.mergeInvitation(held, candidate, lineage)
	})
}

// writeInviteSnapshot validates an invitation, then commits the state next
// derives from the held state with the invitation's lineage.
func (s *SOHost) writeInviteSnapshot(
	ctx context.Context,
	candidate *SOState,
	lineage []*SOConfigChange,
	next func(held *SOState) (*SOState, error),
) error {
	// Validate the new checkpoint and lineage before opening the provider's
	// replacement boundary.
	if s.syncFuncs.CheckpointLock == nil || len(candidate.GetConfig().GetConfigChainHash()) == 0 {
		return ErrConfigHistoryUnavailable
	}
	if err := candidate.Validate(s.sharedObjectID); err != nil {
		return err
	}
	if err := candidate.ValidateAuthority(s.sharedObjectID); err != nil {
		return err
	}
	if _, err := VerifyConfigLineage(s.sharedObjectID, candidate.GetConfig(), lineage); err != nil {
		return errors.Wrap(err, "invitation config lineage")
	}

	// Commit state, its checkpoint and its lineage under one provider lock.
	lock, err := s.syncFuncs.CheckpointLock(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lock.Release()
	state, err := next(lock.GetSOState())
	if err != nil {
		return err
	}
	return lock.WriteSOState(ctx, state, lineage...)
}

// mergeInvitation returns the held state advanced to the invitation's config,
// which the lineage must reach from the held config head. The held state keeps
// its operations and invitations; the invitation's invites replace the held
// invites of the same ID.
func (s *SOHost) mergeInvitation(held, candidate *SOState, lineage []*SOConfigChange) (*SOState, error) {
	// Authenticate the invitation as an extension of the held config.
	suffix, err := unappliedConfigChanges(held.GetConfig().GetConfigChainHash(), lineage)
	if err != nil {
		return nil, err
	}
	if err := VerifyConfigChainSuffix(s.sharedObjectID, held.GetConfig(), candidate.GetConfig(), suffix); err != nil {
		return nil, errors.Wrap(err, "invitation configuration authority")
	}

	// Merge the invitation into a copy of the held state.
	next := held.Clone()
	if err := next.mergeSnapshot(s.sharedObjectID, candidate); err != nil {
		return nil, err
	}

	// Replace held invites by ID and append new ones.
	byID := make(map[string]int, len(next.Invites))
	for i, invite := range next.Invites {
		byID[invite.GetInviteId()] = i
	}
	for _, invite := range candidate.Clone().GetInvites() {
		if idx, ok := byID[invite.GetInviteId()]; ok {
			next.Invites[idx] = invite
			continue
		}
		byID[invite.GetInviteId()] = len(next.Invites)
		next.Invites = append(next.Invites, invite)
	}

	// Check the merged state.
	if err := next.Validate(s.sharedObjectID); err != nil {
		return nil, errors.Wrap(err, "invitation state")
	}
	return next, nil
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

	// An owner handing over signs nothing more on the transfer head.
	if err := s.refuseDepartingSigners(ctx, entry); err != nil {
		return err
	}

	// Commit the change and its associated state in one provider write.
	return s.UpdateSOState(ctx, func(next *SOState) error {
		// Verify the transition against the configuration held under the lock.
		config, err := VerifyConfigChange(s.sharedObjectID, next.GetConfig(), entry)
		if err != nil {
			return err
		}
		next.Config = config
		next.pruneControlMessages()

		// Include the caller's associated state changes.
		if fn == nil {
			return nil
		}
		return fn(next)
	}, entry)
}

// retryConfigChange runs change, which reads the config and signs a change of
// it, until no concurrent writer moves the config head between that read and
// the apply. Each retry waits for the host to publish a state other than the
// one held before the failed attempt.
func retryConfigChange[T any](ctx context.Context, host *SOHost, change func() (T, error)) (T, error) {
	// Watch the state so a retry follows the head that invalidated the change.
	var zero T
	states, release, err := host.GetSOStateCtr(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer release()

	// Run the change until it applies or fails for another reason.
	for {
		held := states.GetValue()
		out, err := change()
		if !errors.Is(err, ErrConfigChainHeadMismatch) {
			return out, err
		}
		if _, err := states.WaitValueChange(ctx, held, nil); err != nil {
			return zero, err
		}
	}
}

// AddLocalOperation signs opData as the owner of privKey and adds it to the
// operation set, encoded with the key of the epoch the operation names. It
// returns the operation's local ID and nonce once the state holding it is
// written. Empty opData writes an acknowledgment. A write whose outcome
// cannot be settled returns ErrStateWriteUnknown: the state may hold the
// operation, so a caller looks for it before adding it again.
func (s *SOHost) AddLocalOperation(
	ctx context.Context,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	privKey crypto.PrivKey,
	opData []byte,
) (string, uint64, error) {
	// Serialize nonce selection and admission under the provider lock.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", 0, err
	}
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return "", 0, err
	}
	defer lk.Release()

	// Encode the data for the head of this peer's chain. An acknowledgment
	// has no data to encode.
	next := lk.GetSOState().Clone()
	set, err := s.verifier.OperationSet(next)
	if err != nil {
		return "", 0, err
	}
	link := next.NextOperationLink(set, peerID.String())
	var opDataEnc []byte
	if len(opData) != 0 {
		handle := NewSOStateParticipantHandle(le, sfs, s.sharedObjectID, next, privKey, peerID).WithGrantKeyring(s.keyring)
		xfrm, err := handle.epochTransformer(link.KeyEpoch)
		if err != nil {
			return "", 0, err
		}
		opDataEnc, err = xfrm.EncodeBlock(opData)
		if err != nil {
			return "", 0, err
		}
	}

	// Sign it, add it to the set and commit.
	localID := NewSOOperationLocalID()
	op, err := BuildSOOperation(s.sharedObjectID, privKey, opDataEnc, link, localID)
	if err != nil {
		return "", 0, err
	}
	if _, err := next.AddOperation(s.sharedObjectID, op); err != nil {
		return "", 0, err
	}

	// Place it in the same write when this peer is the sequencer, so the
	// sequencer never takes the lock again for its own operations.
	if _, err := next.sequenceOperations(s.verifier, privKey); err != nil {
		return "", 0, err
	}
	if err := lk.WriteSOState(ctx, next); err != nil {
		return "", 0, err
	}
	return localID, link.Nonce, nil
}
