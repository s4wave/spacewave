package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RemoveSOParticipants removes participant configs and grants in one signed
// configuration change. It returns the peer IDs that were present and removed.
func RemoveSOParticipants(
	ctx context.Context,
	host *SOHost,
	targetPeerIDs []string,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) ([]string, error) {
	// Collect the requested peers.
	targets := make(map[string]struct{}, len(targetPeerIDs))
	for _, peerID := range targetPeerIDs {
		if peerID != "" {
			targets[peerID] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	// Read the current config.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current SO state")
	}
	currentCfg := state.GetConfig()
	if currentCfg == nil {
		return nil, nil
	}

	// Remove only peers that currently participate.
	var removed []string
	for _, participant := range currentCfg.GetParticipants() {
		if _, ok := targets[participant.GetPeerId()]; ok {
			removed = append(removed, participant.GetPeerId())
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}

	// Sign the removal and prune the removed peers' grants with it.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(participant *SOParticipantConfig) bool {
		_, ok := targets[participant.GetPeerId()]
		return ok
	})
	entry, err := BuildSOConfigChange(host.GetSharedObjectID(), currentCfg, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, signerPriv, revInfo)
	if err != nil {
		return nil, errors.Wrap(err, "build config change")
	}
	if err := host.ApplyConfigChange(ctx, entry, func(state *SOState) error {
		return pruneRemovedParticipants(host.GetSharedObjectID(), state, targets, signerPriv)
	}); err != nil {
		return nil, err
	}
	return removed, nil
}

// RemoveSOParticipant removes one participant config and grant.
func RemoveSOParticipant(
	ctx context.Context,
	host *SOHost,
	targetPeerID string,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) (bool, error) {
	removed, err := RemoveSOParticipants(ctx, host, []string{targetPeerID}, signerPriv, revInfo)
	return len(removed) != 0, err
}

// pruneRemovedParticipants removes the targets' grants from every key epoch
// and replaces their proofs that remaining participants depend on, signed by
// the remaining owner of signer. Their operations stay in the set, and replay
// skips them.
func pruneRemovedParticipants(sharedObjectID string, state *SOState, targets map[string]struct{}, signer crypto.PrivKey) error {
	// Drop the targets' grants and re-wrap the grants they signed.
	signerID, err := peer.IDFromPrivateKey(signer)
	if err != nil {
		return err
	}
	current := state.CurrentKeyEpoch().GetEpoch()
	for _, epoch := range state.GetKeyEpochs() {
		epoch.Grants = slices.DeleteFunc(epoch.Grants, func(grant *SOGrant) bool {
			_, ok := targets[grant.GetPeerId()]
			return ok
		})
		if err := rewrapRemovedGrants(sharedObjectID, epoch, targets, signer, signerID.String(), epoch.GetEpoch() == current); err != nil {
			return errors.Wrapf(err, "key epoch %d", epoch.GetEpoch())
		}
	}

	// Co-sign the held checkpoint when no remaining owner signed it.
	checkpoint := state.GetCheckpoint()
	if checkpoint == nil {
		return nil
	}
	_, signers, err := checkpoint.Verify(sharedObjectID)
	if err != nil {
		return err
	}
	for i, signer := range slices.Backward(signers) {
		if _, removed := targets[signer]; removed {
			checkpoint.Signatures = slices.Delete(checkpoint.Signatures, i, i+1)
		}
	}
	if _, err := checkpoint.ValidateAuthority(sharedObjectID, state.GetConfig().GetParticipants()); err == nil {
		return nil
	}
	return checkpoint.CoSign(signer)
}

// rewrapRemovedGrants re-encrypts, as signer, each grant in epoch that a
// target signed. Without its own grant to the epoch's key, signer drops those
// grants instead, unless the epoch is the current one.
func rewrapRemovedGrants(sharedObjectID string, epoch *SOKeyEpoch, targets map[string]struct{}, signer crypto.PrivKey, signerID string, required bool) error {
	var inner *SOGrantInner
	for i := 0; i < len(epoch.Grants); i++ {
		// Keep a grant whose signer remains.
		grant := epoch.Grants[i]
		grantSigner, err := grant.Verify(sharedObjectID)
		if err != nil {
			return err
		}
		if _, removed := targets[grantSigner]; !removed {
			continue
		}

		// Read the epoch key from signer's own grant.
		if inner == nil {
			own := epoch.FindGrant(signerID)
			if own == nil {
				if required {
					return errors.New("remaining owner grant required to replace removed signer grants")
				}
				epoch.Grants = slices.Delete(epoch.Grants, i, i+1)
				i--
				continue
			}
			if inner, err = own.DecryptInnerData(signer, sharedObjectID); err != nil {
				return err
			}
		}

		// Wrap it for the recipient.
		_, recipient, err := peer.ParsePeerIDWithPubKey(grant.GetPeerId())
		if err != nil {
			return err
		}
		if epoch.Grants[i], err = EncryptSOGrant(signer, recipient, sharedObjectID, inner); err != nil {
			return err
		}
	}
	return nil
}
