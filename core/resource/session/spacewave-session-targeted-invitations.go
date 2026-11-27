package resource_session

import (
	"context"
	cryptorand "crypto/rand"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ResolveUsername resolves an exact username for an allowed invite context.
func (r *SpacewaveSessionResource) ResolveUsername(
	ctx context.Context,
	req *s4wave_provider_spacewave.ResolveUsernameRequest,
) (*s4wave_provider_spacewave.ResolveUsernameResponse, error) {
	// Translate the requested username-resolution purpose to the Cloud API enum.
	var purpose api.TargetedInvitePurpose
	switch req.GetPurpose() {
	case s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE:
		purpose = api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE
	case s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION:
		purpose = api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION
	default:
		purpose = api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_UNSPECIFIED
	}

	// Resolve the exact username within its invite context.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.ResolveUsername(ctx, &api.ResolveUsernameRequest{
		Username: req.GetUsername(),
		Purpose:  purpose,
		SpaceId:  req.GetSpaceId(),
		OrgId:    req.GetOrgId(),
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.ResolveUsernameResponse{
		Found:        resp.GetFound(),
		AccountId:    resp.GetAccountId(),
		EntityId:     resp.GetEntityId(),
		DomainId:     resp.GetDomainId(),
		Relationship: resp.GetRelationship(),
		CanInvite:    resp.GetCanInvite(),
		EntityUuid:   resp.GetEntityUuid(),
		AccountEpoch: resp.GetAccountEpoch(),
	}, nil
}

// CreateTargetedInvitation creates a signed pending targeted invitation.
func (r *SpacewaveSessionResource) CreateTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateTargetedInvitationRequest,
) (*s4wave_provider_spacewave.CreateTargetedInvitationResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.CreateTargetedInvitation(ctx, &api.CreateTargetedInvitationRequest{
		TargetAccountId: req.GetTargetAccountId(),
		Purpose:         targetedInvitePurposeToAPI(req.GetPurpose()),
		SpaceId:         req.GetSpaceId(),
		OrgId:           req.GetOrgId(),
		Role:            req.GetRole(),
		ExpiresAt:       req.GetExpiresAt(),
		Envelope:        targetedInvitationEnvelopeToAPI(req.GetEnvelope()),
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.CreateTargetedInvitationResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// CreateSpaceTargetedInvitationByUsername creates a Space targeted invitation
// by exact username and stores it in the recipient inbox.
func (r *SpacewaveSessionResource) CreateSpaceTargetedInvitationByUsername(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateSpaceTargetedInvitationByUsernameRequest,
) (*s4wave_provider_spacewave.CreateSpaceTargetedInvitationByUsernameResponse, error) {
	// Validate the request and resolve the role.
	username := req.GetUsername()
	spaceID := req.GetSpaceId()
	if username == "" {
		return nil, errors.New("username is required")
	}
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	roleName := req.GetRole()
	if roleName == "" {
		roleName = "reader"
	}
	role, err := targetedSpaceRoleFromString(roleName)
	if err != nil {
		return nil, err
	}

	// Resolve the username to an account the cloud lets this Space invite.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resolve, err := cli.ResolveUsername(ctx, &api.ResolveUsernameRequest{
		Username: username,
		Purpose:  api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE,
		SpaceId:  spaceID,
	})
	if err != nil {
		return nil, err
	}
	if !resolve.GetFound() || !resolve.GetCanInvite() || resolve.GetAccountId() == "" {
		return nil, errors.New("username is not available for this space invite")
	}

	// Read the inviting account and mount the Space's invite host.
	account, err := cli.GetAccountInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get account info")
	}
	ih, rel, err := r.parent.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	defer rel()

	// Sign a single-use invite for the resolved account.
	inviteMsg, err := ih.GetSOHost().CreateSOInviteOp(ctx, ih.GetPrivKey(), ih.GetProviderID(), &sobject.SOInvite{
		Role:            role,
		TargetAccountId: resolve.GetAccountId(),
		MaxUses:         1,
		ExpiresAt:       targetedInvitationExpiresAt(req.GetExpiresAt()),
	})
	if err != nil {
		return nil, errors.Wrap(err, "create targeted space invite")
	}

	// Encode the invite as the envelope payload.
	payload, err := inviteMsg.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal space invite payload")
	}

	// Build and sign the envelope under a fresh nonce.
	nonce := make([]byte, 16)
	if _, err := cryptorand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "generate nonce")
	}
	envelope := &api.TargetedInvitationEnvelope{
		SchemaVersion:      1,
		Purpose:            api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE,
		ContextId:          spaceID,
		ActorAccountId:     account.GetAccountId(),
		ActorEntityUuid:    account.GetEntityUuid(),
		ActorAccountEpoch:  int64(account.GetEpoch()),
		SignerPeerId:       cli.GetPeerID().String(),
		TargetAccountId:    resolve.GetAccountId(),
		TargetEntityId:     resolve.GetEntityId(),
		TargetEntityUuid:   resolve.GetEntityUuid(),
		TargetAccountEpoch: resolve.GetAccountEpoch(),
		Role:               roleName,
		ExpiresAt:          req.GetExpiresAt(),
		Nonce:              nonce,
		Payload:            payload,
	}
	if envelope.ActorEntityUuid == "" {
		envelope.ActorEntityUuid = envelope.ActorAccountId
	}
	if err := cli.SignTargetedInvitationEnvelope(envelope); err != nil {
		return nil, err
	}

	// Store the invitation in the recipient's inbox.
	resp, err := cli.CreateTargetedInvitation(ctx, &api.CreateTargetedInvitationRequest{
		TargetAccountId: envelope.GetTargetAccountId(),
		Purpose:         api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE,
		SpaceId:         spaceID,
		Role:            roleName,
		ExpiresAt:       req.GetExpiresAt(),
		Envelope:        envelope,
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.CreateSpaceTargetedInvitationByUsernameResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// AcceptSpaceTargetedInvitation accepts a pending Space targeted invitation
// through the existing mailbox-backed join path.
func (r *SpacewaveSessionResource) AcceptSpaceTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.AcceptSpaceTargetedInvitationRequest,
) (*s4wave_provider_spacewave.AcceptSpaceTargetedInvitationResponse, error) {
	// Verify the invitation.
	envelope, err := r.getAcceptableTargetedInvitation(ctx, req.GetId(), api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE)
	if err != nil {
		return nil, err
	}

	// Decode the Space invite it carries.
	inviteMsg := &sobject.SOInviteMessage{}
	if err := inviteMsg.UnmarshalVT(envelope.GetPayload()); err != nil {
		return nil, errors.Wrap(err, "unmarshal targeted space invite payload")
	}
	if inviteMsg.GetSharedObjectId() != envelope.GetContextId() {
		return nil, errors.New("targeted space invite payload context mismatch")
	}

	// Encode the envelope for the join request.
	envelopeBytes, err := envelope.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal targeted invitation envelope")
	}

	// Join the Space through its mailbox.
	joinResp, err := r.parent.JoinSpaceViaInvite(ctx, &s4wave_session.JoinSpaceViaInviteRequest{
		InviteMessage:              inviteMsg,
		TargetedInvitationEnvelope: envelopeBytes,
	})
	if err != nil {
		return nil, err
	}
	switch joinResp.GetResult() {
	case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED,
		s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL:
	default:
		return nil, errors.New("targeted space invite mailbox submission was not accepted")
	}

	// Mark the invitation accepted.
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Accept the targeted Space invitation through the current signer.
	processResp, err := cli.ProcessTargetedInvitation(ctx, &api.ProcessTargetedInvitationRequest{
		Id:     req.GetId(),
		Action: "accept",
	})
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateTargetedInvitations()
	return &s4wave_provider_spacewave.AcceptSpaceTargetedInvitationResponse{
		Invitation:     targetedInvitationInfoFromAPI(processResp.GetInvitation()),
		SharedObjectId: joinResp.GetSharedObjectId(),
		JoinResult:     targetedSpaceJoinResultString(joinResp.GetResult()),
	}, nil
}

// CreateOrganizationTargetedInvitationByUsername creates an organization
// targeted invitation by exact username and stores it in the recipient inbox.
func (r *SpacewaveSessionResource) CreateOrganizationTargetedInvitationByUsername(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateOrganizationTargetedInvitationByUsernameRequest,
) (*s4wave_provider_spacewave.CreateOrganizationTargetedInvitationByUsernameResponse, error) {
	// Validate the recipient, organization, and supported member role.
	username := req.GetUsername()
	orgID := req.GetOrgId()
	if username == "" {
		return nil, errors.New("username is required")
	}
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	roleName := req.GetRole()
	if roleName == "" {
		roleName = "org:member"
	}
	if roleName != "org:member" {
		return nil, errors.New("only org:member targeted invitations are supported")
	}

	// Resolve the username and account allowed to receive the organization invite.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resolve, err := cli.ResolveUsername(ctx, &api.ResolveUsernameRequest{
		Username: username,
		Purpose:  api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION,
		OrgId:    orgID,
	})
	if err != nil {
		return nil, err
	}
	if !resolve.GetFound() || !resolve.GetCanInvite() {
		return nil, errors.New("username is not available for this organization invite")
	}
	account, err := cli.GetAccountInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get account info")
	}

	// Build and sign the organization invitation envelope for the resolved account.
	nonce := make([]byte, 16)
	if _, err := cryptorand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "generate nonce")
	}
	envelope := &api.TargetedInvitationEnvelope{
		SchemaVersion:      1,
		Purpose:            api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION,
		ContextId:          orgID,
		ActorAccountId:     account.GetAccountId(),
		ActorEntityUuid:    account.GetEntityUuid(),
		ActorAccountEpoch:  int64(account.GetEpoch()),
		SignerPeerId:       cli.GetPeerID().String(),
		TargetAccountId:    resolve.GetAccountId(),
		TargetEntityId:     resolve.GetEntityId(),
		TargetEntityUuid:   resolve.GetEntityUuid(),
		TargetAccountEpoch: resolve.GetAccountEpoch(),
		Role:               roleName,
		ExpiresAt:          req.GetExpiresAt(),
		Nonce:              nonce,
		Payload:            []byte("organization-targeted-invite-v1"),
	}
	if envelope.ActorEntityUuid == "" {
		envelope.ActorEntityUuid = envelope.ActorAccountId
	}
	if err := cli.SignTargetedInvitationEnvelope(envelope); err != nil {
		return nil, err
	}

	// Store the signed organization invitation in the recipient inbox.
	resp, err := cli.CreateTargetedInvitation(ctx, &api.CreateTargetedInvitationRequest{
		TargetAccountId: envelope.GetTargetAccountId(),
		Purpose:         api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION,
		OrgId:           orgID,
		Role:            roleName,
		ExpiresAt:       req.GetExpiresAt(),
		Envelope:        envelope,
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.CreateOrganizationTargetedInvitationByUsernameResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// AcceptOrganizationTargetedInvitation accepts a pending organization targeted
// invitation and mirrors the membership into local org state.
func (r *SpacewaveSessionResource) AcceptOrganizationTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.AcceptOrganizationTargetedInvitationRequest,
) (*s4wave_provider_spacewave.AcceptOrganizationTargetedInvitationResponse, error) {
	// Verify the invitation.
	envelope, err := r.getAcceptableTargetedInvitation(ctx, req.GetId(), api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION)
	if err != nil {
		return nil, err
	}

	// Join the organization.
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Accept the targeted organization invitation through the current signer.
	acceptResp, err := cli.AcceptTargetedOrganizationInvitation(
		ctx, envelope.GetContextId(), &api.AcceptTargetedOrganizationInvitationRequest{Id: req.GetId()},
	)
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateTargetedInvitations()
	org := acceptResp.GetOrganization()
	if org == nil {
		return nil, errors.New("targeted organization invite response missing organization")
	}

	// Mirror the membership into local organization state.
	if err := r.swAcc.RefreshSharedObjectList(ctx); err != nil {
		r.le.WithError(err).Warn("failed to refresh SO list after targeted org join")
	}
	r.refreshOrganizationCaches(ctx, org.GetId(), true)
	r.queueOrgUpdateOp(ctx, org.GetId(), &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_AddMember{
			AddMember: &s4wave_org.AddOrgMember{
				Member: &s4wave_org.OrgMemberInfo{
					AccountId:   r.swAcc.GetAccountID(),
					DisplayRole: s4wave_org.OrgRoleMember,
					JoinedAt:    timestamppb.Now(),
				},
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.AcceptOrganizationTargetedInvitationResponse{
		Invitation: targetedInvitationInfoFromAPI(acceptResp.GetInvitation()),
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:          org.GetId(),
			DisplayName: org.GetDisplayName(),
			Role:        org.GetRole(),
			SpaceIds:    org.GetSpaceIds(),
		},
	}, nil
}

// ListTargetedInvitations lists the caller's targeted invitation inbox.
func (r *SpacewaveSessionResource) ListTargetedInvitations(
	ctx context.Context,
	req *s4wave_provider_spacewave.ListTargetedInvitationsRequest,
) (*s4wave_provider_spacewave.ListTargetedInvitationsResponse, error) {
	resp, _, err := r.swAcc.GetTargetedInvitations(ctx)
	if err != nil {
		return nil, err
	}
	return targetedInvitationsFromAPI(resp), nil
}

// WatchTargetedInvitations streams recipient targeted invitation inbox snapshots.
func (r *SpacewaveSessionResource) WatchTargetedInvitations(
	req *s4wave_provider_spacewave.ListTargetedInvitationsRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchTargetedInvitationsStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_provider_spacewave.ListTargetedInvitationsResponse
	for {
		// Read the cached inbox and send it when it changed.
		apiResp, ch, err := r.swAcc.GetTargetedInvitations(ctx)
		if err != nil {
			return err
		}
		resp := targetedInvitationsFromAPI(apiResp)
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		// Wait for the inbox to change.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// targetedInvitationsFromAPI maps an inbox response to its resource form.
func targetedInvitationsFromAPI(
	resp *api.ListTargetedInvitationsResponse,
) *s4wave_provider_spacewave.ListTargetedInvitationsResponse {
	invitations := make([]*s4wave_provider_spacewave.TargetedInvitationInfo, 0, len(resp.GetInvitations()))
	for _, inv := range resp.GetInvitations() {
		invitations = append(invitations, targetedInvitationInfoFromAPI(inv))
	}
	return &s4wave_provider_spacewave.ListTargetedInvitationsResponse{
		Invitations: invitations,
	}
}

// GetTargetedInvitation reads one targeted invitation.
func (r *SpacewaveSessionResource) GetTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetTargetedInvitationRequest,
) (*s4wave_provider_spacewave.GetTargetedInvitationResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.GetTargetedInvitation(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.GetTargetedInvitationResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// RevokeTargetedInvitation revokes one pending targeted invitation.
func (r *SpacewaveSessionResource) RevokeTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.RevokeTargetedInvitationRequest,
) (*s4wave_provider_spacewave.RevokeTargetedInvitationResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.RevokeTargetedInvitation(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RevokeTargetedInvitationResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// ProcessTargetedInvitation applies a recipient lifecycle action.
func (r *SpacewaveSessionResource) ProcessTargetedInvitation(
	ctx context.Context,
	req *s4wave_provider_spacewave.ProcessTargetedInvitationRequest,
) (*s4wave_provider_spacewave.ProcessTargetedInvitationResponse, error) {
	// Apply the action, then refetch the changed inbox.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.ProcessTargetedInvitation(ctx, &api.ProcessTargetedInvitationRequest{
		Id:     req.GetId(),
		Action: req.GetAction(),
	})
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateTargetedInvitations()
	return &s4wave_provider_spacewave.ProcessTargetedInvitationResponse{
		Invitation: targetedInvitationInfoFromAPI(resp.GetInvitation()),
	}, nil
}

// getAcceptableTargetedInvitation fetches the targeted invitation with id and
// checks that it is pending, of purpose, addressed to this account and signed
// by its creator. It returns the invitation's envelope.
func (r *SpacewaveSessionResource) getAcceptableTargetedInvitation(
	ctx context.Context,
	id string,
	purpose api.TargetedInvitePurpose,
) (*api.TargetedInvitationEnvelope, error) {
	// Fetch the invitation.
	if id == "" {
		return nil, errors.New("id is required")
	}

	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Fetch the targeted invitation before validating its envelope.
	resp, err := cli.GetTargetedInvitation(ctx, id)
	if err != nil {
		return nil, err
	}
	inv := resp.GetInvitation()
	if inv == nil {
		return nil, errors.New("targeted invitation is missing")
	}

	// Check the invitation row.
	if inv.GetStatus() != "pending" {
		return nil, errors.New("targeted invitation is not pending")
	}
	if inv.GetPurpose() != purpose {
		return nil, errors.Errorf("targeted invitation purpose is %v, want %v", inv.GetPurpose(), purpose)
	}

	// Check that the envelope matches the row.
	envelope := inv.GetEnvelope()
	switch {
	case envelope == nil:
		return nil, errors.New("targeted invitation envelope is required")
	case envelope.GetSchemaVersion() != 1:
		return nil, errors.New("unsupported targeted invitation envelope version")
	case envelope.GetPurpose() != inv.GetPurpose() || envelope.GetContextId() != inv.GetContextId():
		return nil, errors.New("targeted invitation envelope context mismatch")
	case envelope.GetTargetAccountId() != inv.GetTargetAccountId() ||
		envelope.GetTargetEntityId() != inv.GetTargetEntityId() ||
		envelope.GetTargetEntityUuid() != inv.GetTargetEntityUuid() ||
		envelope.GetTargetAccountEpoch() != inv.GetTargetAccountEpoch():
		return nil, errors.New("targeted invitation envelope target mismatch")
	case envelope.GetRole() != inv.GetRole() || envelope.GetExpiresAt() != inv.GetExpiresAt():
		return nil, errors.New("targeted invitation envelope row mismatch")
	case len(envelope.GetNonce()) == 0:
		return nil, errors.New("targeted invitation envelope is incomplete")
	case envelope.GetExpiresAt() > 0 && envelope.GetExpiresAt() <= time.Now().UnixMilli():
		return nil, errors.New("targeted invitation is expired")
	}

	// Check the purpose's role and payload.
	switch purpose {
	case api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE:
		if len(envelope.GetPayload()) == 0 {
			return nil, errors.New("targeted invitation envelope is incomplete")
		}
	case api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION:
		if envelope.GetRole() != "org:member" {
			return nil, errors.New("targeted invitation envelope role mismatch")
		}
	}

	// Check the creator's signature.
	if err := provider_spacewave.VerifyTargetedInvitationEnvelope(envelope); err != nil {
		return nil, err
	}

	// Resolve the current signer before reading the account.
	cli, err = r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Check that the envelope addresses this account.
	account, err := cli.GetAccountInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get account info")
	}
	entityUUID := account.GetEntityUuid()
	if entityUUID == "" {
		entityUUID = account.GetAccountId()
	}
	if envelope.GetTargetAccountId() != account.GetAccountId() || envelope.GetTargetEntityUuid() != entityUUID {
		return nil, errors.New("targeted invitation is not for this account")
	}
	return envelope, nil
}

// targetedSpaceJoinResultString maps the mailbox join outcome to the invitation response.
func targetedSpaceJoinResultString(result s4wave_session.JoinSpaceViaInviteResult) string {
	switch result {
	case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED:
		return "accepted"
	case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL:
		return "pending_owner_approval"
	case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_REJECTED:
		return "rejected"
	case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE:
		return "owner_must_be_online"
	default:
		return "unspecified"
	}
}

// targetedSpaceRoleFromString resolves the supported Space invitation roles.
func targetedSpaceRoleFromString(role string) (sobject.SOParticipantRole, error) {
	switch role {
	case "reader", "":
		return sobject.SOParticipantRole_SOParticipantRole_READER, nil
	case "writer":
		return sobject.SOParticipantRole_SOParticipantRole_WRITER, nil
	default:
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, errors.New("unsupported space invite role")
	}
}

// targetedInvitationExpiresAt converts a positive expiry to a timestamp.
func targetedInvitationExpiresAt(expiresAt int64) *timestamppb.Timestamp {
	if expiresAt <= 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(expiresAt))
}

// targetedInvitePurposeToAPI maps the resource invitation purpose to the cloud API.
func targetedInvitePurposeToAPI(purpose s4wave_provider_spacewave.TargetedInvitePurpose) api.TargetedInvitePurpose {
	switch purpose {
	case s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE:
		return api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE
	case s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION:
		return api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION
	default:
		return api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_UNSPECIFIED
	}
}

// targetedInvitePurposeFromAPI maps the cloud invitation purpose to the resource API.
func targetedInvitePurposeFromAPI(purpose api.TargetedInvitePurpose) s4wave_provider_spacewave.TargetedInvitePurpose {
	switch purpose {
	case api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE:
		return s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE
	case api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION:
		return s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_ORGANIZATION
	default:
		return s4wave_provider_spacewave.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_UNSPECIFIED
	}
}

// targetedInvitationEnvelopeToAPI copies the signed envelope into its cloud representation.
func targetedInvitationEnvelopeToAPI(in *s4wave_provider_spacewave.TargetedInvitationEnvelope) *api.TargetedInvitationEnvelope {
	if in == nil {
		return nil
	}
	return &api.TargetedInvitationEnvelope{
		SchemaVersion:      in.GetSchemaVersion(),
		Purpose:            targetedInvitePurposeToAPI(in.GetPurpose()),
		ContextId:          in.GetContextId(),
		ActorAccountId:     in.GetActorAccountId(),
		ActorEntityUuid:    in.GetActorEntityUuid(),
		ActorAccountEpoch:  in.GetActorAccountEpoch(),
		SignerPeerId:       in.GetSignerPeerId(),
		TargetAccountId:    in.GetTargetAccountId(),
		TargetEntityId:     in.GetTargetEntityId(),
		TargetEntityUuid:   in.GetTargetEntityUuid(),
		TargetAccountEpoch: in.GetTargetAccountEpoch(),
		Role:               in.GetRole(),
		ExpiresAt:          in.GetExpiresAt(),
		Nonce:              in.GetNonce(),
		Payload:            in.GetPayload(),
		Signature:          in.GetSignature(),
	}
}

// targetedInvitationEnvelopeFromAPI copies the signed envelope into its resource representation.
func targetedInvitationEnvelopeFromAPI(in *api.TargetedInvitationEnvelope) *s4wave_provider_spacewave.TargetedInvitationEnvelope {
	if in == nil {
		return nil
	}
	return &s4wave_provider_spacewave.TargetedInvitationEnvelope{
		SchemaVersion:      in.GetSchemaVersion(),
		Purpose:            targetedInvitePurposeFromAPI(in.GetPurpose()),
		ContextId:          in.GetContextId(),
		ActorAccountId:     in.GetActorAccountId(),
		ActorEntityUuid:    in.GetActorEntityUuid(),
		ActorAccountEpoch:  in.GetActorAccountEpoch(),
		SignerPeerId:       in.GetSignerPeerId(),
		TargetAccountId:    in.GetTargetAccountId(),
		TargetEntityId:     in.GetTargetEntityId(),
		TargetEntityUuid:   in.GetTargetEntityUuid(),
		TargetAccountEpoch: in.GetTargetAccountEpoch(),
		Role:               in.GetRole(),
		ExpiresAt:          in.GetExpiresAt(),
		Nonce:              in.GetNonce(),
		Payload:            in.GetPayload(),
		Signature:          in.GetSignature(),
	}
}

// targetedInvitationInfoFromAPI projects the cloud invitation into the resource response.
func targetedInvitationInfoFromAPI(in *api.TargetedInvitationInfo) *s4wave_provider_spacewave.TargetedInvitationInfo {
	if in == nil {
		return nil
	}
	return &s4wave_provider_spacewave.TargetedInvitationInfo{
		Id:                 in.GetId(),
		ActorAccountId:     in.GetActorAccountId(),
		TargetAccountId:    in.GetTargetAccountId(),
		TargetEntityId:     in.GetTargetEntityId(),
		TargetEntityUuid:   in.GetTargetEntityUuid(),
		TargetAccountEpoch: in.GetTargetAccountEpoch(),
		Purpose:            targetedInvitePurposeFromAPI(in.GetPurpose()),
		ContextId:          in.GetContextId(),
		Role:               in.GetRole(),
		Status:             in.GetStatus(),
		EnvelopeHash:       in.GetEnvelopeHash(),
		Envelope:           targetedInvitationEnvelopeFromAPI(in.GetEnvelope()),
		CreatedAt:          in.GetCreatedAt(),
		UpdatedAt:          in.GetUpdatedAt(),
		ExpiresAt:          in.GetExpiresAt(),
	}
}
