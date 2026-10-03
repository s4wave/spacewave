package sobject

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// SequencerHost is an optional interface on SharedObject implementations whose
// local peer can choose the sequencer as an owner.
type SequencerHost interface {
	// SetSequencer appoints peerID as the sequencer from the sequence the local
	// peer holds, or selects Merge when peerID is empty. It reports false when
	// peerID already sequences.
	SetSequencer(ctx context.Context, peerID string) (bool, error)
}

// MainDevice is an optional interface on SharedObject implementations whose
// local peer signs positions when the config appoints it, as the main device
// of a Space that syncs between devices.
type MainDevice interface {
	// SequenceOperations places every operation the sequence has not placed
	// while the local peer is the sequencer.
	SequenceOperations(ctx context.Context) error
}

// SetSOSequencer signs, as the owner of signer, one configuration change that
// appoints peerID as the sequencer, or selects Merge when peerID is empty. The
// new sequencer starts after the last position the local peer resolves, so the
// order it already holds stays. Moving to this device and replacing a lost
// device are both this change: positions the old sequencer signed after the
// start are ignored and their operations placed again. It reports false
// without a change when peerID already sequences.
func SetSOSequencer(ctx context.Context, host *SOHost, peerID string, signer crypto.PrivKey) (bool, error) {
	// Read the current config and sequence.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, errors.Wrap(err, "get current SO state")
	}
	current := state.GetConfig()
	if current.GetSequencer().GetPeerId() == peerID {
		return false, nil
	}
	set, err := state.OperationSet(host.GetSharedObjectID())
	if err != nil {
		return false, err
	}

	// Sign and apply the change.
	nextCfg := current.CloneVT()
	nextCfg.Sequencer = &SOSequencer{PeerId: peerID, Start: set.SequenceTail()}
	entry, err := BuildSOConfigChange(host.GetSharedObjectID(), current, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SET_SEQUENCER, signer, nil)
	if err != nil {
		return false, errors.Wrap(err, "build config change")
	}
	if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
		return false, err
	}
	return true, nil
}

// SequenceOperations places, as the sequencer of privKey, every operation the
// sequence has not placed, in one write. It writes nothing unless privKey is
// the appointed sequencer and an operation awaits a position.
func (s *SOHost) SequenceOperations(ctx context.Context, privKey crypto.PrivKey) error {
	// Hold the provider lock through placement and persistence.
	lk, err := s.lockFn(ctx, s.sharedObjectID)
	if err != nil {
		return err
	}
	defer lk.Release()

	// Sign the positions on a copy and commit them if any.
	next := lk.GetSOState().CloneVT()
	added, err := next.SequenceOperations(s.sharedObjectID, privKey)
	if err != nil || len(added) == 0 {
		return err
	}
	return lk.WriteSOState(ctx, next)
}

// Sequence calls place each time the state changes while the local peer is
// the appointed sequencer, so operations from this device and from its peers
// get positions as they arrive. The positions reach the other devices with the
// state they sync. It watches the state and returns when ctx ends or place
// fails.
func Sequence(ctx context.Context, so SharedObject, place func(context.Context) error) error {
	// Watch the state as the local peer.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	peerID := so.GetPeerID().String()

	// Place the operations of each state while appointed.
	var snap SharedObjectStateSnapshot
	for {
		// Wait for the next state.
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		if snap == nil {
			continue
		}

		// Only the appointed sequencer places operations.
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		if cfg.GetSequencer().GetPeerId() != peerID {
			continue
		}
		if err := place(ctx); err != nil {
			return err
		}
	}
}
