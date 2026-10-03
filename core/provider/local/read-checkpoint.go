package provider_local

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

// readCheckpointKey addresses private history, outside replicated SOState.
func readCheckpointKey(sharedObjectID string) []byte {
	return []byte("so/" + sharedObjectID + "/read-checkpoint")
}

// writeReadCheckpoint retains the last readable state at the authority commit
// boundary. A peer whose leave the head ownership transfer
// carries is no longer readable. Readmission removes the checkpoint because the
// current World owns history again. It runs after the commit's configuration
// history is written to tx.
func writeReadCheckpoint(
	ctx context.Context,
	tx kvtx.Tx,
	sharedObjectID string,
	localPeer peer.ID,
	previous, next *sobject.SOState,
) error {
	// Skip commits that do not change this peer's readability.
	if localPeer == "" {
		return nil
	}
	if head := next.GetConfig().GetConfigChainHash(); len(head) != 0 && bytes.Equal(head, previous.GetConfig().GetConfigChainHash()) {
		return nil
	}

	// Detect whether this peer can read each state.
	readable := func(state *sobject.SOState) (bool, error) {
		// The role must grant reads.
		participants := state.GetConfig().GetParticipants()
		index := slices.IndexFunc(participants, func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == localPeer.String()
		})
		if index == -1 || !sobject.CanReadState(participants[index].GetRole()) {
			return false, nil
		}

		// The head transition must not carry this peer's leave.
		head := state.GetConfig().GetConfigChainHash()
		if len(head) == 0 {
			return true, nil
		}
		entry, err := readSOConfigEntry(ctx, tx, sharedObjectID, head)
		if err != nil {
			return false, err
		}
		departing, err := sobject.SODepartingPeers(entry)
		return !slices.Contains(departing, localPeer.String()), err
	}
	wasReadable, err := readable(previous)
	if err != nil {
		return err
	}
	isReadable, err := readable(next)
	if err != nil || wasReadable == isReadable {
		return err
	}

	// Delete the checkpoint on readmission; retain the readable state on departure.
	key := readCheckpointKey(sharedObjectID)
	if isReadable {
		return tx.Delete(ctx, key)
	}
	data, err := previous.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, key, data)
}

// GetSharedObjectReadCheckpoint returns the state retained at departure.
// A nil checkpoint means this participant can still read the shared object.
func (s *SharedObject) GetSharedObjectReadCheckpoint(ctx context.Context) (*sobject.SharedObjectReadCheckpoint, error) {
	// Read the state retained at departure.
	read, err := s.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer read.Discard()
	data, found, err := read.Get(ctx, readCheckpointKey(s.GetSharedObjectID()))
	if err != nil {
		return nil, err
	}
	if found {
		state := &sobject.SOState{}
		if err := state.UnmarshalVT(data); err != nil {
			return nil, err
		}
		return s.buildReadCheckpoint(state), nil
	}

	// Without a recorded departure, an authoritative denial ends access at the
	// current state.
	current, err := s.soHost.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	if !sobject.AuthoritativeSyncDenied(current.GetConfig(), s.tkr.healthCtr.GetValue()) {
		return nil, nil
	}
	return s.buildReadCheckpoint(current.CloneVT()), nil
}

// buildReadCheckpoint returns the read checkpoint of a retained state.
func (s *SharedObject) buildReadCheckpoint(state *sobject.SOState) *sobject.SharedObjectReadCheckpoint {
	return &sobject.SharedObjectReadCheckpoint{
		Config:   state.GetConfig(),
		Snapshot: s.lsoHost.buildSnapshot(state),
	}
}
