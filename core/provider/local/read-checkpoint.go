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

// writeReadCheckpoint retains the last readable root at the authority commit boundary.
// A peer whose leave the head ownership transfer carries is no longer readable.
// Readmission removes the checkpoint because the current World owns history again.
// It runs after the commit's configuration history is written to tx.
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
