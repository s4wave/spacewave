package resource_session

import (
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ApproveGuestSpaceLink approves an independently keyed guest without registering
// a Session under the approving cloud account. The caller must authorize the
// guest's application identity before invoking this operation. The signed ticket
// proves possession of the guest key; it does not prove application membership.
// The returned invite is restricted to that key and permits one redemption.
func (r *SpacewaveSessionResource) ApproveGuestSpaceLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.ApproveSpaceLinkRequest,
) (*sobject.SOInviteMessage, error) {
	// Verify the guest's signed request before accessing the selected Space.
	now := time.Now()
	verified, err := verifySpaceLinkTicketData(req.GetTicket(), now)
	if err != nil {
		return nil, err
	}
	resourceID := string(req.GetResourceId())
	if resourceID == "" {
		return nil, errors.New("resource_id is required")
	}

	// Only an unlocked OWNER Session may issue an independent guest grant.
	key := r.session.GetPrivKey()
	if key == nil {
		return nil, errors.New("session is locked")
	}
	shared, release, err := r.mountSpaceSO(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := requireSpaceLinkApproverOwner(ctx, shared, r.session.GetPeerId().String()); err != nil {
		return nil, err
	}

	// Consume the signed request before issuing its capability. A failed write
	// requires a fresh ticket, so retrying cannot mint another one-use invite.
	payload := verified.payload
	if err := r.swAcc.ConsumeSpaceLinkNonce(
		ctx, payload.GetAgentPeerId(), payload.GetNonce(),
		verified.ticket.GetPayload(), time.Unix(payload.GetExpiresAt(), 0),
	); err != nil {
		return nil, err
	}

	// Use the invitation owner so the signed grant is also registered with
	// the cloud mailbox. A bare SO operation cannot admit an offline guest.
	invite, err := r.parent.CreateSpaceInvite(ctx, &s4wave_session.CreateSpaceInviteRequest{
		SpaceId:      resourceID,
		Role:         payload.GetRequestedRole(),
		TargetPeerId: verified.agentPeerID.String(),
		MaxUses:      1,
		ExpiresAt:    timestamppb.New(now.Add(localSpaceLinkInviteTTL)),
	})
	if err != nil {
		return nil, err
	}
	return invite.GetInviteMessage(), nil
}
