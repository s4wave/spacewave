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
	if err := VerifyConfigChainSuffix(s.sharedObjectID, previous.GetConfig(), candidate.GetConfig(), changes); err != nil {
		return errors.Wrap(err, "peer snapshot configuration authority")
	}
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
		next := previous.CloneVT()
		next.Config = candidate.GetConfig().CloneVT()
		next.KeyEpochs = nil
		next.Ops = nil
		next.ControlMessages = nil
		if err := lock.WriteSOState(ctx, next, changes...); err != nil {
			return err
		}
		return ErrParticipantRevoked
	}

	// Adopt the verified config and the candidate's checkpoint. Invitations
	// stay locally administered.
	next := previous.CloneVT()
	next.Config = candidate.GetConfig().CloneVT()
	next.pruneControlMessages()
	if checkpoint := candidate.GetCheckpoint(); checkpoint != nil {
		if err := next.AdoptCheckpoint(s.sharedObjectID, checkpoint); err != nil {
			return errors.Wrap(err, "peer snapshot checkpoint")
		}
	}

	// Merge the grants, operations, sequence and control messages.
	if err := next.MergeKeyEpochs(s.sharedObjectID, candidate.GetKeyEpochs()); err != nil {
		return errors.Wrap(err, "peer snapshot key epochs")
	}
	for _, op := range candidate.GetOps() {
		if _, err := next.AddOperation(s.sharedObjectID, op); err != nil {
			return errors.Wrap(err, "peer snapshot operation")
		}
	}
	for _, record := range candidate.GetSequence() {
		if _, err := next.AddSequence(s.sharedObjectID, record); err != nil {
			return errors.Wrap(err, "peer snapshot sequence")
		}
	}
	next.MergeControlMessages(s.sharedObjectID, candidate.GetControlMessages())

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

// InstallInviteSnapshot installs an explicit, externally authenticated
// invitation result with lineage, the inviter's retained changes leading to
// its config, oldest first. The lineage lets this replica resolve the config
// of every operation the candidate holds. The caller must have authenticated
// the invitation owner and its response before calling this operation.
// Ordinary peer synchronization must use ImportPeerSnapshot.
func (s *SOHost) InstallInviteSnapshot(ctx context.Context, candidate *SOState, lineage []*SOConfigChange) error {
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
	return lock.WriteSOState(ctx, candidate.CloneVT(), lineage...)
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
	nextState.Config, err = VerifyConfigChange(s.sharedObjectID, nextState.GetConfig(), entry)
	if err != nil {
		return err
	}
	nextState.pruneControlMessages()

	// Include associated state changes in the same provider write.
	if fn != nil {
		if err := fn(nextState); err != nil {
			return err
		}
	}

	return lk.WriteSOState(ctx, nextState, entry)
}

// AddLocalOperation signs opData as the owner of privKey and adds it to the
// operation set, encoded with the key of the epoch the operation names. It
// returns the operation's local ID and nonce once the state holding it is
// written. Empty opData writes an acknowledgment.
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
	next := lk.GetSOState().CloneVT()
	link, err := next.NextOperationLink(s.sharedObjectID, peerID.String())
	if err != nil {
		return "", 0, err
	}
	var opDataEnc []byte
	if len(opData) != 0 {
		handle := NewSOStateParticipantHandle(le, sfs, s.sharedObjectID, next, privKey, peerID)
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
	if err := lk.WriteSOState(ctx, next); err != nil {
		return "", 0, err
	}
	return localID, link.Nonce, nil
}
