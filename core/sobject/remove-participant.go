package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// RemoveSOParticipant removes a target peer's participant config and grant
// from a single shared object.
//
// Returns true if the participant was found and removed, false if they were
// not a participant. Builds a signed SOConfigChange removing the participant
// and atomically removes the corresponding SOGrant from RootGrants. Does not
// rotate the transform key; control-plane revocation (P2P block DEX and cloud
// block exchange denial) handles access removal.
//
// signerPriv must be the private key of an OWNER in the current config.
func RemoveSOParticipant(
	ctx context.Context,
	host *SOHost,
	targetPeerIDStr string,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) (bool, error) {
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, errors.Wrap(err, "get current SO state")
	}

	currentCfg := state.GetConfig()
	if currentCfg == nil {
		return false, nil
	}

	// Check if participant exists.
	found := slices.ContainsFunc(currentCfg.GetParticipants(), func(p *SOParticipantConfig) bool {
		return p.GetPeerId() == targetPeerIDStr
	})
	if !found {
		return false, nil
	}

	// Build the new config with the participant removed.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(p *SOParticipantConfig) bool {
		return p.GetPeerId() == targetPeerIDStr
	})

	entry, err := BuildSOConfigChange(currentCfg, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, signerPriv, revInfo)
	if err != nil {
		return false, errors.Wrap(err, "build config change")
	}

	// Apply config change and remove grant atomically.
	err = host.ApplyConfigChange(ctx, entry, func(st *SOState) error {
		st.RootGrants = slices.DeleteFunc(st.RootGrants, func(g *SOGrant) bool {
			return g.GetPeerId() == targetPeerIDStr
		})
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
