package sobject

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RemoveSOParticipants removes participant configs and grants in one signed
// configuration change. It returns the peer IDs that were present and removed.
// Under group control signerPriv, a voter, agrees to the removal and it
// returns those peers with ErrAwaitingGroup; the voters take the key from them
// once the group decides.
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

	// Sign the removal, dropping the removed peers from the roster's drops, and
	// prune their grants with it.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(participant *SOParticipantConfig) bool {
		_, ok := targets[participant.GetPeerId()]
		return ok
	})
	nextCfg.RosterDroppedPeerIds = slices.DeleteFunc(nextCfg.RosterDroppedPeerIds, func(peerID string) bool {
		_, ok := targets[peerID]
		return ok
	})
	if err := pinRemovedAuthors(host.GetSharedObjectID(), state, nextCfg, removed); err != nil {
		return nil, err
	}
	err = ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, signerPriv, revInfo, func(state *SOState) error {
		return pruneRemovedParticipants(host.GetSharedObjectID(), state, targets, signerPriv)
	})
	if err != nil && !errors.Is(err, ErrAwaitingGroup) {
		return nil, err
	}
	return removed, err
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

// pinRemovedAuthors records in next, the config that removes peers, the last
// operation state holds from each removed peer that its checkpoint does not
// cover. It drops pins the checkpoint now covers and pins of peers next
// admits again, and keeps the list sorted by peer ID.
func pinRemovedAuthors(sharedObjectID string, state *SOState, next *SharedObjectConfig, peers []string) error {
	// Read the operations held above the checkpoint.
	set, err := state.OperationSet(sharedObjectID)
	if err != nil {
		return err
	}

	// Keep each earlier pin a later checkpoint or rejoin has not made redundant.
	pins := make(map[string]*SOOperationPosition, len(next.GetRemovedAuthors())+len(peers))
	for _, pin := range next.GetRemovedAuthors() {
		participates := slices.ContainsFunc(next.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == pin.GetPeerId() })
		if !participates && !set.Covers(pin.GetPeerId(), pin.GetNonce()) {
			pins[pin.GetPeerId()] = pin
		}
	}

	// Pin each removed peer's latest held operation above the checkpoint.
	for _, peerID := range peers {
		nonce, head := set.AuthorHead(peerID)
		if nonce != 0 && !set.Covers(peerID, nonce) {
			pins[peerID] = &SOOperationPosition{PeerId: peerID, Nonce: nonce, OpHash: head}
		}
	}

	// Store them in peer ID order.
	next.RemovedAuthors = slices.SortedFunc(maps.Values(pins), func(a, b *SOOperationPosition) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	return nil
}

// pruneRemovedParticipants removes the targets' grants from every key epoch
// and replaces their proofs that remaining participants depend on, signed by
// the remaining owner of signer. It revokes every invite that names a target,
// since a named peer may redeem its invite again. Their operations stay in the
// set, and replay applies only those the removal pinned.
func pruneRemovedParticipants(sharedObjectID string, state *SOState, targets map[string]struct{}, signer crypto.PrivKey) error {
	// Close the invites that would readmit a target.
	for _, invite := range state.GetInvites() {
		if _, ok := targets[invite.GetTargetPeerId()]; ok {
			invite.Revoked = true
		}
	}

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
	if _, err := checkpoint.ValidateAuthority(sharedObjectID, state.GetConfig()); err == nil {
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
