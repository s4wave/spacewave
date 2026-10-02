package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
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
		return pruneRemovedParticipants(host.sharedObjectID, state, currentCfg, targets, signerPriv)
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

// pruneRemovedParticipants removes the targets' grants and replaces or drops
// every state proof the targets signed, so the held state remains valid under
// the next configuration already set on state. currentCfg is the configuration
// before the change and signerPriv is a remaining owner's key.
func pruneRemovedParticipants(
	sharedObjectID string,
	state *SOState,
	currentCfg *SharedObjectConfig,
	targets map[string]struct{},
	signerPriv crypto.PrivKey,
) error {
	nextCfg := state.GetConfig()

	// Pending operations and signed outcomes must remain admissible under the
	// next configuration. Account nonces stay reserved so a remaining peer
	// cannot reuse the nonce of a dropped rejection.
	state.Ops = slices.DeleteFunc(state.Ops, func(op *SOOperation) bool {
		return op.ValidateSignature(sharedObjectID, nextCfg.GetParticipants()) != nil
	})
	rejections := state.OpRejections[:0]
	for _, group := range state.OpRejections {
		group.Rejections = slices.DeleteFunc(group.Rejections, func(rejection *SOOperationRejection) bool {
			_, err := rejection.ValidateSignature(sharedObjectID, nextCfg.GetParticipants())
			return err != nil
		})
		if len(group.Rejections) != 0 {
			rejections = append(rejections, group)
		}
	}
	state.OpRejections = rejections

	// Retained readers need proofs from a remaining validator. Grant
	// encryption binds its signer, so replacing a signature also requires
	// wrapping the unchanged transform key for each affected recipient.
	signerID, err := peer.IDFromPrivateKey(signerPriv)
	if err != nil {
		return err
	}
	removedSigner := func(signature *peer.Signature) (bool, error) {
		pub, err := signature.ParsePubKey()
		if err != nil {
			return false, err
		}
		if pub == nil {
			return false, peer.ErrEmptyPeerID
		}
		id, err := peer.IDFromPublicKey(pub)
		if err != nil {
			return false, err
		}
		_, removed := targets[id.String()]
		return removed, nil
	}
	state.RootGrants = slices.DeleteFunc(state.RootGrants, func(grant *SOGrant) bool {
		_, ok := targets[grant.GetPeerId()]
		return ok
	})
	var inner *SOGrantInner
	for i, grant := range state.RootGrants {
		removed, err := removedSigner(grant.GetSignature())
		if err != nil {
			return err
		}
		if !removed {
			continue
		}
		if inner == nil {
			for _, ownGrant := range state.RootGrants {
				if ownGrant.GetPeerId() == signerID.String() {
					if err := ownGrant.ValidateSignature(sharedObjectID, currentCfg.GetParticipants()); err != nil {
						return err
					}
					inner, err = ownGrant.DecryptInnerData(signerPriv, sharedObjectID)
					if err != nil {
						return err
					}
					break
				}
			}
			if inner == nil {
				return errors.New("remaining owner grant required to replace removed validator proofs")
			}
		}
		_, recipient, err := peer.ParsePeerIDWithPubKey(grant.GetPeerId())
		if err != nil {
			return err
		}
		state.RootGrants[i], err = EncryptSOGrant(signerPriv, recipient, sharedObjectID, inner)
		if err != nil {
			return err
		}
		if err := state.RootGrants[i].ValidateSignature(sharedObjectID, nextCfg.GetParticipants()); err != nil {
			return err
		}
	}

	// A final departure leaves no validator to sign the retained root.
	root := state.GetRoot()
	if len(root.GetInner()) != 0 && len(nextCfg.GetParticipants()) != 0 {
		retained := root.ValidatorSignatures[:0]
		for _, signature := range root.GetValidatorSignatures() {
			removed, err := removedSigner(signature)
			if err != nil {
				return err
			}
			if !removed {
				retained = append(retained, signature)
			}
		}
		root.ValidatorSignatures = retained
		if len(retained) == 0 {
			if err := root.SignInnerData(signerPriv, sharedObjectID, root.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
				return err
			}
		}
		valid, err := root.ValidateSignatures(sharedObjectID, nextCfg.GetParticipants())
		if err != nil {
			return err
		}
		return CheckConsensusAcceptance(nextCfg.GetConsensusMode(), valid)
	}
	return nil
}
