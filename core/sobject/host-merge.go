package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// CheckpointFloor returns the config sequence number the held checkpoint was
// signed under: the lowest fork a merge may absorb a branch from. It returns
// zero without a checkpoint, and the held config's sequence number when the
// checkpoint's config is not retained, so no older fork merges.
func (s *SOHost) CheckpointFloor(ctx context.Context, state *SOState) uint64 {
	// Without a checkpoint any fork above genesis may merge.
	held := state.GetConfig()
	inner, err := state.GetCheckpointInner()
	if err != nil {
		return held.GetConfigChainSeqno()
	}
	if inner == nil {
		return 0
	}

	// Resolve the checkpoint's config to its sequence number.
	hash := inner.GetConfigHash()
	if bytes.Equal(hash, held.GetConfigChainHash()) {
		return held.GetConfigChainSeqno()
	}
	entry, err := s.ReadConfigEntry(ctx, hash)
	if err != nil {
		return held.GetConfigChainSeqno()
	}
	return entry.GetConfigSeqno()
}

// ReadConfigBranch returns the retained changes above floor that lead to the
// config head target, oldest first, and the hash before them. See
// ReadConfigBranch.
func (s *SOHost) ReadConfigBranch(ctx context.Context, target []byte, floor uint64) ([]byte, []*SOConfigChange, error) {
	return ReadConfigBranch(ctx, target, floor, s.ReadConfigEntry)
}

// checkMergeFloor rejects a suffix that starts at a fork below the held
// checkpoint, so a merge cannot absorb history the checkpoint made stable.
func (s *SOHost) checkMergeFloor(ctx context.Context, state *SOState, changes []*SOConfigChange) error {
	if len(changes) == 0 || bytes.Equal(changes[0].GetPreviousHash(), state.GetConfig().GetConfigChainHash()) {
		return nil
	}
	if changes[0].GetConfigSeqno() <= s.CheckpointFloor(ctx, state) {
		return errors.Wrap(ErrConfigHistoryUnavailable, "merge forks below the held checkpoint")
	}
	return nil
}

// MergePeerBranch commits a candidate whose config chain diverged from the
// held one. cursor names the peer's record at the held checkpoint floor, and
// changes lead from it to the candidate's config. The newest record both chains
// share is the fork. When the candidate's head is chosen over the held head
// and localKey owns it, this signs a merge record on that head absorbing the
// held branch and commits the candidate with the merge. Otherwise the held
// state is unchanged: the peer whose head loses merges, and an owner merges
// for a reader. A fork below the floor returns ErrConfigHistoryUnavailable.
func (s *SOHost) MergePeerBranch(
	ctx context.Context,
	candidate *SOState,
	cursor []byte,
	changes []*SOConfigChange,
	localKey crypto.PrivKey,
	validateAccess func(context.Context, *SOState) error,
) error {
	// Lock the held state for the import.
	localPeer, err := peer.IDFromPrivateKey(localKey)
	if err != nil {
		return err
	}
	lock, err := s.lockPeerImport(ctx, candidate, changes)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Read the held branch above the checkpoint floor.
	previous := lock.GetSOState()
	held := previous.GetConfig()
	heldCursor, heldBranch, err := s.ReadConfigBranch(ctx, held.GetConfigChainHash(), s.CheckpointFloor(ctx, previous))
	if err != nil {
		return err
	}

	// Find the fork: the newest peer record on the held branch.
	ours, theirs, fork, err := s.splitConfigBranches(ctx, heldCursor, heldBranch, cursor, changes)
	if err != nil {
		return err
	}
	if len(theirs) == 0 {
		return nil
	}
	if len(ours) == 0 {
		if err := VerifyConfigChainSuffix(s.sharedObjectID, held, candidate.GetConfig(), theirs); err != nil {
			return errors.Wrap(err, "peer snapshot configuration authority")
		}
		return s.commitPeerSnapshot(ctx, lock, candidate, theirs, localPeer, validateAccess)
	}

	// Leave the merge to an owner of the winning head when it is not ours.
	target := candidate.GetConfig()
	wins := MergeWins(target.GetConfigChainSeqno(), target.GetConfigChainHash(), held.GetConfigChainSeqno(), held.GetConfigChainHash())
	if !wins || target.IsGroupControl() || !isOwnerPeer(target, localPeer.String()) {
		return nil
	}

	// Authenticate the peer's branch from the fork.
	forkConfig, err := appliedConfig(fork)
	if err != nil {
		return err
	}
	if err := VerifyConfigChainSuffix(s.sharedObjectID, forkConfig, target, theirs); err != nil {
		return errors.Wrap(err, "peer branch configuration authority")
	}

	// Absorb the held branch in a merge on the peer's head and commit both.
	merge, err := BuildSOConfigMerge(s.sharedObjectID, target, fork, ours, localKey)
	if err != nil {
		return err
	}
	merged := candidate.Clone()
	if merged.Config, err = VerifyConfigChange(s.sharedObjectID, target, merge); err != nil {
		return err
	}
	return s.commitPeerSnapshot(ctx, lock, merged, append(slices.Clone(theirs), merge), localPeer, validateAccess)
}

// splitConfigBranches finds the fork, the newest record of the peer's branch
// that the held branch also holds, and returns the records after it on each.
// Each branch starts after its cursor, the record at the same floor.
func (s *SOHost) splitConfigBranches(
	ctx context.Context,
	heldCursor []byte,
	heldBranch []*SOConfigChange,
	peerCursor []byte,
	peerBranch []*SOConfigChange,
) (ours, theirs []*SOConfigChange, fork *SOConfigChange, err error) {
	// Index the held branch by hash; the cursor is position -1.
	heldAt := map[string]int{string(heldCursor): -1}
	for i, change := range heldBranch {
		hash, err := HashSOConfigChange(change)
		if err != nil {
			return nil, nil, nil, err
		}
		heldAt[string(hash)] = i
	}

	// Find the newest peer record the held branch holds.
	oursAt, theirsAt, found := 0, 0, false
	if i, ok := heldAt[string(peerCursor)]; ok {
		oursAt, theirsAt, found = i, -1, true
	}
	for j, change := range peerBranch {
		hash, err := HashSOConfigChange(change)
		if err != nil {
			return nil, nil, nil, err
		}
		if i, ok := heldAt[string(hash)]; ok {
			oursAt, theirsAt, found = i, j, true
		}
	}
	if !found {
		return nil, nil, nil, errors.Wrap(ErrConfigHistoryUnavailable, "config branches share no record above the held checkpoint")
	}

	// Return the record at the fork with both branches after it.
	ours, theirs = heldBranch[oursAt+1:], peerBranch[theirsAt+1:]
	if oursAt != -1 {
		return ours, theirs, heldBranch[oursAt], nil
	}
	if len(ours) != 0 && len(theirs) != 0 {
		fork, err = s.ReadConfigEntry(ctx, heldCursor)
	}
	return ours, theirs, fork, err
}
