package resource_session

import (
	"context"
	"time"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// PreviewSpaceLink verifies a SpaceLink ticket for trusted UI display.
func (r *SpacewaveSessionResource) PreviewSpaceLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.PreviewSpaceLinkRequest,
) (*s4wave_provider_spacewave.PreviewSpaceLinkResponse, error) {
	// Verify the SpaceLink ticket before exposing its trusted preview.
	verified, err := verifySpaceLinkTicketData(req.GetTicket(), time.Now())
	if err != nil {
		return nil, err
	}
	payload := verified.payload
	if err := r.swAcc.CheckSpaceLinkNonceFresh(
		ctx,
		payload.GetAgentPeerId(),
		payload.GetNonce(),
		verified.ticket.GetPayload(),
	); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.PreviewSpaceLinkResponse{
		Label:          payload.GetLabel(),
		SessionType:    payload.GetSessionType(),
		AgentPeerId:    payload.GetAgentPeerId(),
		RequestedRole:  payload.GetRequestedRole(),
		TargetHint:     payload.GetTargetHint(),
		Nonce:          payload.GetNonce(),
		ExpiresAt:      payload.GetExpiresAt(),
		CallbackUrl:    payload.GetCallbackUrl(),
		CompletionMode: payload.GetCompletionMode(),
	}, nil
}

// ApproveSpaceLink approves a SpaceLink ticket for a target Space.
func (r *SpacewaveSessionResource) ApproveSpaceLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.ApproveSpaceLinkRequest,
) (*s4wave_provider_spacewave.ApproveSpaceLinkResponse, error) {
	// Verify the SpaceLink ticket before resolving its target Space.
	verified, err := verifySpaceLinkTicketData(req.GetTicket(), time.Now())
	if err != nil {
		return nil, err
	}
	resourceID := string(req.GetResourceId())
	if resourceID == "" {
		return nil, errors.New("resource_id is required")
	}

	// Mount the target Space and retain its release handle.
	swSO, relSO, err := r.mountSpaceSO(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	defer relSO()

	// Approve the verified SpaceLink through the account's entity client.
	return approveVerifiedSpaceLink(
		ctx,
		verified,
		req.GetResourceId(),
		r.session.GetPeerId().String(),
		r.swAcc,
		r.swAcc,
		mountedSpaceLinkTargetSpace{swSO: swSO},
	)
}

// unmarshalSpaceLinkAuthTicket decodes the ticket and requires its signed payload.
func unmarshalSpaceLinkAuthTicket(data []byte) (*s4wave_provider_spacewave.SpaceLinkAuthTicket, error) {
	// Validate and decode the SpaceLink authorization ticket.
	if len(data) == 0 {
		return nil, errors.New("ticket is required")
	}
	ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{}
	if err := ticket.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal spacelink auth ticket")
	}
	if len(ticket.GetPayload()) == 0 {
		return nil, errors.New("ticket payload is required")
	}
	if len(ticket.GetAgentSignature()) == 0 {
		return nil, errors.New("ticket agent signature is required")
	}
	return ticket, nil
}
