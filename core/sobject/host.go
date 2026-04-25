package sobject

import (
	"bytes"
	"context"

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
	// sharedObjectID is the id of the shared object.
	sharedObjectID string
	// soRc contains the shared object refcount instance.
	soRc *refcount.RefCount[ccontainer.Watchable[*SOState]]
}

// NewSOHost constructs a new shared object host.
//
// ctx can be nil
func NewSOHost(ctx context.Context, watchFn SOStateWatchFunc, lockFn SOStateLockFunc, sharedObjectID string) *SOHost {
	h := &SOHost{watchFn: watchFn, lockFn: lockFn, sharedObjectID: sharedObjectID}
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

// GetSOStateCtr watches the shared object state with the refcount container.
func (s *SOHost) GetSOStateCtr(ctx context.Context, released func()) (ccontainer.Watchable[*SOState], func(), error) {
	return s.soRc.ResolveWithReleased(ctx, released)
}

// GetHostState returns a snapshot of the current SOState.
func (s *SOHost) GetHostState(ctx context.Context) (*SOState, error) {
	watchable, rel, err := s.soRc.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

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
	sr, err := s.GetRootState(ctx)
	if err != nil {
		return nil, nil, err
	}

	sri := &SORootInner{}
	if err := sri.UnmarshalVT(sr.GetInner()); err != nil {
		return nil, sr, err
	}
	return sri, sr, sri.Validate()
}

// UpdateSOState locks the SO state, clones it, calls the provided function
// to mutate the clone, then writes the updated state.
func (s *SOHost) UpdateSOState(ctx context.Context, fn func(state *SOState) error) error {
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	nextState := lk.GetSOState().CloneVT()
	if err := fn(nextState); err != nil {
		return err
	}
	return lk.WriteSOState(ctx, nextState)
}

// UpdateRootState locks the host state and applies the UpdateRootState operation.
//
// If an error is returned the SOState should be considered invalid.
// If enforceValidatorPeerID is non-empty, ensures the given validator is in the set of signatures.
func (s *SOHost) UpdateRootState(
	ctx context.Context,
	nextRootState *SORoot,
	enforceValidatorPeerID string,
	rejectedOps []*SOOperationRejection,
	acceptedOps []*SOOperation,
) error {
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// load and clone the previous state
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// apply the change to nextState
	err = nextState.UpdateRootState(s.sharedObjectID, nextRootState, enforceValidatorPeerID, rejectedOps, acceptedOps)
	if err != nil {
		return err
	}

	// write the change
	return lk.WriteSOState(ctx, nextState)
}

// ClearRejectedOperation clears a rejected operation from the state.
// The clear operation must be signed by the peer that submitted the original operation.
func (s *SOHost) ClearRejectedOperation(ctx context.Context, clearOp *SOClearOperationResult) error {
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// load and clone the previous state
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// apply the change to nextState
	if err := nextState.ClearOperationResult(s.sharedObjectID, clearOp); err != nil {
		return err
	}

	// write the change
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
	if entry == nil {
		return errors.New("config change entry is nil")
	}

	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	currentCfg := nextState.GetConfig()
	currentHash := currentCfg.GetConfigChainHash()
	currentSeqno := currentCfg.GetConfigChainSeqno()

	// Verify previous_hash chains from the current config.
	if !bytes.Equal(entry.GetPreviousHash(), currentHash) {
		return errors.New("config change previous_hash does not match current config_chain_hash")
	}

	// Verify config_seqno is the expected next value.
	var expectedSeqno uint64
	if len(currentHash) != 0 {
		expectedSeqno = currentSeqno + 1
	}
	if entry.GetConfigSeqno() != expectedSeqno {
		return errors.Errorf("config change seqno %d does not match expected %d", entry.GetConfigSeqno(), expectedSeqno)
	}

	// Verify the signature is authorized by the current config.
	if err := verifyConfigChangeSignature(entry, currentCfg); err != nil {
		return errors.Wrap(err, "verify config change")
	}

	// Apply the new config from the entry (clone to avoid mutating the input).
	nextState.Config = entry.GetConfig().CloneVT()

	// Compute and store the new config_chain_hash and seqno.
	entryHash, err := HashSOConfigChange(entry)
	if err != nil {
		return errors.Wrap(err, "hash config change entry")
	}
	nextState.GetConfig().ConfigChainHash = entryHash
	nextState.GetConfig().ConfigChainSeqno = entry.GetConfigSeqno()

	if fn != nil {
		if err := fn(nextState); err != nil {
			return err
		}
	}

	return lk.WriteSOState(ctx, nextState)
}

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
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// load and clone the previous state
	prevState := lk.GetSOState()
	nextState := prevState.CloneVT()

	// determine the next nonce
	nextAccNonce := nextState.GetNextAccountNonce(peerID.String())

	// call the callback
	op, err := cb(nextAccNonce)
	if err != nil {
		return err
	}

	// apply the operation to nextState
	err = nextState.QueueOperation(s.sharedObjectID, op)
	if err != nil {
		return err
	}

	// write the change
	return lk.WriteSOState(ctx, nextState)
}
