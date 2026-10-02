package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// AddSOParticipant adds a target peer as a participant on a single shared
// object and issues an encrypted grant for the peer.
//
// Reads the current SOState, checks for duplicate participant, builds a signed
// SOConfigChange adding the participant, and applies it atomically with a new
// SOGrant encrypted to the target's public key.
//
// Returns the newly created SOGrant for the target peer, or nil if the
// participant already existed (no-op).
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

	// Build the signed config change that adds the participant.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = append(nextCfg.Participants, &SOParticipantConfig{
		PeerId:   targetPeerIDStr,
		Role:     role,
		EntityId: entityID,
		Username: username,
	})
	entry, err := BuildSOConfigChange(soID, currentCfg, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, localPriv, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build config change")
	}

	// Apply config change and issue grant atomically.
	var grant *SOGrant
	err = host.ApplyConfigChange(ctx, entry, func(st *SOState) error {
		// Find the local peer's root grant.
		grants := st.GetRootGrants()
		localGrantIdx := slices.IndexFunc(grants, func(g *SOGrant) bool {
			return g.GetPeerId() == localPeerIDStr
		})
		if localGrantIdx == -1 {
			return errors.New("local grant not found")
		}

		// Re-encrypt the local grant's inner data for the target peer.
		grantInner, err := grants[localGrantIdx].DecryptInnerData(localPriv, soID)
		if err != nil {
			return errors.Wrap(err, "decrypt local grant")
		}
		grant, err = EncryptSOGrant(localPriv, targetPub, soID, grantInner)
		if err != nil {
			return errors.Wrap(err, "encrypt grant for target peer")
		}
		st.RootGrants = append(st.RootGrants, grant)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return grant, nil
}
