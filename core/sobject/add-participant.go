package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// AddSOParticipant adds a target peer as a participant on a single shared
// object and grants it every key epoch the local peer holds, so it can replay
// operations written before a rotation. A peer that is already a participant
// at a lower role is raised to role; one that holds role or a higher one is
// left unchanged and nothing is written. A role is never lowered.
//
// It returns the target's grant for the current key epoch when it added the
// peer, else nil; the outcome; and the role the peer holds after the call.
// Under group control localPriv, a voter, agrees to the addition or raise and
// it returns ErrAwaitingGroup; a writer of an entity without a voter gets one
// vote, and the voters hand a new peer the key once the group decides.
//
// localPriv must be the private key of an OWNER in the current config, which
// the host checks when it verifies the change. localPeerIDStr is the base58
// peer ID corresponding to localPriv. username is the provider username of
// entityID, or empty when entityID is empty. A concurrent local change of the
// config does not fail the addition.
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
) (*SOGrant, SOParticipantAddOutcome, SOParticipantRole, error) {
	res, err := retryConfigChange(ctx, host, func() (soParticipantAdd, error) {
		return addSOParticipant(ctx, host, soID, localPriv, localPeerIDStr, targetPeerIDStr, targetPub, role, entityID, username)
	})
	return res.grant, res.outcome, res.held, err
}

// soParticipantAdd is the result of one AddSOParticipant attempt.
type soParticipantAdd struct {
	grant   *SOGrant
	outcome SOParticipantAddOutcome
	held    SOParticipantRole
}

// addSOParticipant makes one attempt at AddSOParticipant from the config the
// host holds now.
func addSOParticipant(
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
) (soParticipantAdd, error) {
	// Reject roles a participant add may not grant.
	if err := ValidateSOParticipantRole(role, false); err != nil {
		return soParticipantAdd{}, err
	}

	// Read the current config from the host state.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return soParticipantAdd{}, errors.Wrap(err, "get current SO state")
	}
	currentCfg := state.GetConfig()
	if currentCfg == nil {
		currentCfg = &SharedObjectConfig{}
	}

	// Leave a participant at role or above, and raise one below it.
	held := currentCfg.GetParticipantRole(targetPeerIDStr)
	if held >= role {
		return soParticipantAdd{outcome: SOParticipantAddOutcome_SOParticipantAddOutcome_UNCHANGED, held: held}, nil
	}
	if held != SOParticipantRole_SOParticipantRole_UNKNOWN {
		return raiseSOParticipant(ctx, host, state, localPriv, targetPeerIDStr, role)
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
		return soParticipantAdd{}, err
	}
	return soParticipantAdd{grant: grant, outcome: SOParticipantAddOutcome_SOParticipantAddOutcome_ADDED, held: role}, nil
}

// raiseSOParticipant raises the role of targetPeerIDStr, a participant below
// role, to role. Its grants stay as they are: every participant reads the key.
func raiseSOParticipant(
	ctx context.Context,
	host *SOHost,
	state *SOState,
	localPriv crypto.PrivKey,
	targetPeerIDStr string,
	role SOParticipantRole,
) (soParticipantAdd, error) {
	// Raise the role, giving a new writer a vote under group control when its
	// entity has none.
	nextCfg := state.GetConfig().CloneVT()
	for _, p := range nextCfg.GetParticipants() {
		if p.GetPeerId() != targetPeerIDStr {
			continue
		}
		p.Role = role
		if nextCfg.IsGroupControl() && CanWriteOps(role) && p.GetVotingWeight() == 0 && !entityVotes(nextCfg, p.GetEntityId()) {
			p.VotingWeight = 1
		}
	}

	// Sign and apply the change; the host rejects a signer that is not an owner.
	err := ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, localPriv, nil, nil)
	if err != nil {
		return soParticipantAdd{}, err
	}
	return soParticipantAdd{outcome: SOParticipantAddOutcome_SOParticipantAddOutcome_RAISED, held: role}, nil
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
