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
	targets := make(map[string]struct{}, len(targetPeerIDs))
	for _, peerID := range targetPeerIDs {
		if peerID != "" {
			targets[peerID] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	state, err := host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current SO state")
	}
	currentCfg := state.GetConfig()
	if currentCfg == nil {
		return nil, nil
	}

	var removed []string
	for _, participant := range currentCfg.GetParticipants() {
		if _, ok := targets[participant.GetPeerId()]; ok {
			removed = append(removed, participant.GetPeerId())
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}

	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(participant *SOParticipantConfig) bool {
		_, ok := targets[participant.GetPeerId()]
		return ok
	})
	entry, err := BuildSOConfigChange(currentCfg, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, signerPriv, revInfo)
	if err != nil {
		return nil, errors.Wrap(err, "build config change")
	}
	if err := host.ApplyConfigChange(ctx, entry, func(state *SOState) error {
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
						if err := ownGrant.ValidateSignature(host.sharedObjectID, currentCfg.GetParticipants()); err != nil {
							return err
						}
						inner, err = ownGrant.DecryptInnerData(signerPriv, host.sharedObjectID)
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
			state.RootGrants[i], err = EncryptSOGrant(signerPriv, recipient, host.sharedObjectID, inner)
			if err != nil {
				return err
			}
			if err := state.RootGrants[i].ValidateSignature(host.sharedObjectID, nextCfg.GetParticipants()); err != nil {
				return err
			}
		}

		root := state.GetRoot()
		if len(root.GetInner()) != 0 {
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
				if err := root.SignInnerData(signerPriv, host.sharedObjectID, root.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
					return err
				}
			}
			valid, err := root.ValidateSignatures(host.sharedObjectID, nextCfg.GetParticipants())
			if err != nil {
				return err
			}
			return CheckConsensusAcceptance(nextCfg.GetConsensusMode(), valid)
		}
		return nil
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
