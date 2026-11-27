package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// resolveMemberPeersAndMountSO resolves an account's session peers and
// username, and mounts the space's shared object when the account has peers.
// Caller must defer releaseFn.
func (r *SpacewaveSessionResource) resolveMemberPeersAndMountSO(
	ctx context.Context,
	spaceID, accountID string,
) (*provider_spacewave.SharedObject, func(), *api.EnrollMemberResponse, error) {
	// Ask the Cloud for the member's session peers and username.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, nil, nil, err
	}
	enrollResp, err := cli.EnrollMember(ctx, spaceID, accountID, true)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "resolve member peers")
	}
	if len(enrollResp.GetPeers()) == 0 {
		return nil, func() {}, enrollResp, nil
	}

	// Mount the Space SharedObject to enroll the peers.
	swSO, rel, err := r.mountSpaceSO(ctx, spaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	return swSO, rel, enrollResp, nil
}

// resolveMemberParticipantPeersAndMountSO resolves an account's existing SO
// participant peers and mounts the space's shared object. Caller must defer
// releaseFn.
func (r *SpacewaveSessionResource) resolveMemberParticipantPeersAndMountSO(
	ctx context.Context,
	spaceID, accountID string,
) (*provider_spacewave.SharedObject, func(), []string, error) {
	// Resolve the member's participant peers from the Cloud account.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, nil, nil, err
	}
	resolveResp, err := cli.ResolveMemberParticipants(ctx, spaceID, accountID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "resolve member participant peers")
	}

	// Stop when the account has no existing participant peers.
	peerIDs := resolveResp.GetPeerIds()
	if len(peerIDs) == 0 {
		return nil, func() {}, nil, nil
	}

	// Mount the Space SharedObject that owns those participants.
	swSO, rel, err := r.mountSpaceSO(ctx, spaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	// Return the mounted Space and the resolved member peers.
	return swSO, rel, peerIDs, nil
}

// EnrollSpaceMember enrolls an org member into a space by adding them as a participant.
func (r *SpacewaveSessionResource) EnrollSpaceMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.EnrollSpaceMemberRequest,
) (*s4wave_provider_spacewave.EnrollSpaceMemberResponse, error) {
	// Require the Space and the member account.
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	accountID := req.GetAccountId()
	if accountID == "" {
		return nil, errors.New("account_id is required")
	}

	// Resolve the member's peers and mount the Space SharedObject.
	swSO, relSO, member, err := r.resolveMemberPeersAndMountSO(ctx, spaceID, accountID)
	if err != nil {
		return nil, err
	}
	defer relSO()
	peers := member.GetPeers()
	if len(peers) == 0 {
		return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{}, nil
	}

	// Add each peer, keeping a higher existing role, and record the outcome.
	role := req.GetRole()
	results := make([]*s4wave_provider_spacewave.EnrollSpaceMemberResult, 0, len(peers))
	for _, p := range peers {
		peerID := p.GetPeerId()
		result := &s4wave_provider_spacewave.EnrollSpaceMemberResult{PeerId: peerID}
		targetPub, err := session.ExtractPublicKeyFromPeerID(peerID)
		if err != nil {
			result.Error = errors.Wrap(err, "extract pubkey").Error()
			results = append(results, result)
			continue
		}

		// Add the peer, raising a lower existing role and keeping a higher one.
		_, outcome, held, err := swSO.AddParticipant(ctx, peerID, targetPub, role, accountID, member.GetEntityId())
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		result.Outcome = outcome
		result.ParticipantRole = held
		results = append(results, result)
	}
	return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{Results: results}, nil
}

// RemoveSpaceMember removes an org member from a space by removing them as a participant.
func (r *SpacewaveSessionResource) RemoveSpaceMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.RemoveSpaceMemberRequest,
) (*s4wave_provider_spacewave.RemoveSpaceMemberResponse, error) {
	// Check the request.
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	accountID := req.GetAccountId()
	if accountID == "" {
		return nil, errors.New("account_id is required")
	}

	// Find the member's peers.
	swSO, relSO, peerIDs, err := r.resolveMemberParticipantPeersAndMountSO(ctx, spaceID, accountID)
	if err != nil {
		return nil, err
	}
	defer relSO()
	if len(peerIDs) == 0 {
		return nil, errors.New("no participant peers found for account")
	}

	// Remove every participant peer in one config change, or agree to removal
	// under group control, and report each peer.
	revInfo := &sobject.SORevocationInfo{
		Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_OWNER_REMOVED,
	}
	removed, err := swSO.RemoveParticipantsWithRevocation(ctx, peerIDs, revInfo)
	if errors.Is(err, sobject.ErrAwaitingGroup) {
		return &s4wave_provider_spacewave.RemoveSpaceMemberResponse{AwaitingGroup: true}, nil
	}
	removedSet := make(map[string]struct{}, len(removed))
	for _, peerID := range removed {
		removedSet[peerID] = struct{}{}
	}
	results := make([]*s4wave_provider_spacewave.RemoveSpaceMemberResult, 0, len(peerIDs))
	for _, peerID := range peerIDs {
		result := &s4wave_provider_spacewave.RemoveSpaceMemberResult{PeerId: peerID}
		if err != nil {
			result.Error = err.Error()
		} else {
			_, result.Removed = removedSet[peerID]
			result.NotParticipant = !result.Removed
		}
		results = append(results, result)
	}

	return &s4wave_provider_spacewave.RemoveSpaceMemberResponse{Results: results}, nil
}
