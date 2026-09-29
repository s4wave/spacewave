package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

// readCheckpointKey addresses private history, outside replicated SOState.
func readCheckpointKey(sharedObjectID string) []byte {
	return []byte("so/" + sharedObjectID + "/read-checkpoint")
}

// writeReadCheckpoint retains the last readable root at the authority commit boundary.
// Readmission removes the checkpoint because the current World owns history again.
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

	// Detect whether this peer can read each state.
	readable := func(state *sobject.SOState) bool {
		for _, participant := range state.GetConfig().GetParticipants() {
			if participant.GetPeerId() == localPeer.String() {
				return sobject.CanReadState(participant.GetRole())
			}
		}
		return false
	}
	wasReadable, isReadable := readable(previous), readable(next)
	if wasReadable == isReadable {
		return nil
	}

	// Delete the checkpoint on readmission; retain the readable root on departure.
	key := readCheckpointKey(sharedObjectID)
	if isReadable {
		return tx.Delete(ctx, key)
	}

	// Only this participant's grant is needed to decode the retained root.
	checkpoint := &sobject.SOState{
		Config: previous.GetConfig(),
		Root:   previous.GetRoot(),
	}
	for _, grant := range previous.GetRootGrants() {
		if grant.GetPeerId() == localPeer.String() {
			checkpoint.RootGrants = append(checkpoint.RootGrants, grant)
		}
	}
	data, err := checkpoint.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, key, data)
}

// GetSharedObjectReadCheckpoint returns the last readable snapshot retained at departure.
// A nil snapshot means this provider has no retained history for this participant.
func (s *SharedObject) GetSharedObjectReadCheckpoint(ctx context.Context) (*sobject.SharedObjectReadCheckpoint, error) {
	// Read the retained checkpoint or build one from the current state.
	read, err := s.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer read.Discard()
	data, found, err := read.Get(ctx, readCheckpointKey(s.GetSharedObjectID()))
	if err != nil {
		return nil, err
	}

	// Decode the retained checkpoint or snapshot the current denied state.
	state := &sobject.SOState{}
	if found {
		if err := state.UnmarshalVT(data); err != nil {
			return nil, err
		}
	} else {
		current, err := s.soHost.GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		if !sobject.AuthoritativeSyncDenied(current.GetConfig(), s.tkr.healthCtr.GetValue()) {
			return nil, nil
		}
		state.Config = current.GetConfig().CloneVT()
		state.Root = current.GetRoot().CloneVT()
		for _, grant := range current.GetRootGrants() {
			if grant.GetPeerId() == s.localPid.String() {
				state.RootGrants = append(state.RootGrants, grant.CloneVT())
			}
		}
	}

	// Validate and return the checkpoint handle.
	if err := state.Validate(s.GetSharedObjectID()); err != nil {
		return nil, err
	}
	return &sobject.SharedObjectReadCheckpoint{
		Snapshot: sobject.NewSOStateParticipantHandle(s.lsoHost.le, s.lsoHost.sfs, s.GetSharedObjectID(), state, s.localPriv, s.localPid),
		Config:   state.GetConfig().CloneVT(),
	}, nil
}
