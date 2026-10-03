package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// AddSOParticipant adds a target peer as a participant on a single shared
// object and grants it every key epoch the local peer holds, so it can replay
// operations written before a rotation.
//
// Reads the current SOState, checks for duplicate participant, builds a signed
// SOConfigChange adding the participant, and applies it atomically with the
// grants encrypted to the target's public key.
//
// Returns the target's grant for the current key epoch, or nil if the
// participant already existed (no-op). Under group control localPriv, a
// voter, agrees to the addition and it returns ErrAwaitingGroup; a writer of
// an entity without a voter joins with one vote, and the voters hand it the
// key once the group decides.
//
// localPriv must be the private key of an OWNER in the current config.
// localPeerIDStr is the base58 peer ID corresponding to localPriv. username is
// the provider username of entityID, or empty when entityID is empty.
func AddSOParticipant(
	ctx context.Context,
	host *SOHost,
	soID string,
	localPriv crypto.PrivKey,
	localPeerIDStr string,
	targetPeerIDStr string,
	targetPub crypto.PubKey,
	role SOParticipantRole,
	entityID string,
	username string,
) (*SOGrant, error) {
	// Reject roles a participant add may not grant.
	if err := ValidateSOParticipantRole(role, false); err != nil {
		return nil, err
	}

	// Read the current config from the host state.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current SO state")
	}
	currentCfg := state.GetConfig()
	if currentCfg == nil {
		currentCfg = &SharedObjectConfig{}
	}

	// Skip a peer that is already a participant.
	for _, p := range currentCfg.GetParticipants() {
		if p.GetPeerId() == targetPeerIDStr {
			return nil, nil
		}
	}

	// Add the participant, with a vote under group control when its entity
	// has none.
	added := &SOParticipantConfig{
		PeerId:   targetPeerIDStr,
		Role:     role,
		EntityId: entityID,
		Username: username,
	}
	if currentCfg.IsGroupControl() && CanWriteOps(role) && !entityVotes(currentCfg, entityID) {
		added.VotingWeight = 1
	}
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = append(nextCfg.Participants, added)

	// Apply the config change and issue the grants atomically.
	var grant *SOGrant
	err = ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, localPriv, nil, func(st *SOState) error {
		current := st.CurrentKeyEpoch().GetEpoch()
		for _, held := range st.GetKeyEpochs() {
			// Skip an epoch the local peer cannot read; the current one is required.
			i := slices.IndexFunc(held.GetGrants(), func(g *SOGrant) bool {
				return g.GetPeerId() == localPeerIDStr
			})
			if i == -1 {
				if held.GetEpoch() == current {
					return errors.New("local grant not found")
				}
				continue
			}

			// Re-encrypt the local grant's inner data for the target peer.
			grantInner, err := held.GetGrants()[i].DecryptInnerData(localPriv, soID)
			if err != nil {
				return errors.Wrapf(err, "decrypt local grant for key epoch %d", held.GetEpoch())
			}
			next, err := EncryptSOGrant(localPriv, targetPub, soID, grantInner)
			if err != nil {
				return errors.Wrapf(err, "encrypt key epoch %d grant for target peer", held.GetEpoch())
			}
			held.Grants = append(held.Grants, next)
			if held.GetEpoch() == current {
				grant = next
			}
		}
		if grant == nil {
			return errors.New("local grant not found")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return grant, nil
}

// entityVotes reports whether a participant of entityID votes under cfg. A
// participant without an entity is its own entity.
func entityVotes(cfg *SharedObjectConfig, entityID string) bool {
	if entityID == "" {
		return false
	}
	return slices.ContainsFunc(cfg.GetParticipants(), func(p *SOParticipantConfig) bool {
		return p.GetEntityId() == entityID && p.GetVotingWeight() != 0
	})
}
