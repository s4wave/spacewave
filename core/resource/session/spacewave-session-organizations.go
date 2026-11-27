package resource_session

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/resource/session/rootstate"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchOrganizations streams the user's org list, emitting on membership changes.
func (r *SpacewaveSessionResource) WatchOrganizations(
	req *s4wave_provider_spacewave.WatchOrganizationsRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOrganizationsStream,
) error {
	ctx := strm.Context()
	return r.swAcc.WatchOrgList(ctx, func(list []*api.OrgResponse) {
		orgs := make([]*s4wave_provider_spacewave.OrganizationInfo, len(list))
		for i, o := range list {
			orgs[i] = &s4wave_provider_spacewave.OrganizationInfo{
				Id:          o.GetId(),
				DisplayName: o.GetDisplayName(),
				Role:        o.GetRole(),
				SpaceIds:    o.GetSpaceIds(),
			}
		}
		_ = strm.Send(&s4wave_provider_spacewave.WatchOrganizationsResponse{Organizations: orgs})
	})
}

// CreateOrganization creates a new organization.
func (r *SpacewaveSessionResource) CreateOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateOrganizationRequest,
) (*s4wave_provider_spacewave.CreateOrganizationResponse, error) {
	// Create the Cloud organization and decode its authoritative response.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	data, err := cli.CreateOrganization(ctx, req.GetDisplayName())
	if err != nil {
		return nil, err
	}
	var info api.OrgResponse
	if err := info.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal org")
	}

	// Publish the cloud response synchronously so cached org-list readers cannot
	// observe a stale list after this RPC returns. Root SO bootstrap and the full
	// cloud refresh run through the ProviderAccount keyed org-sync routine.
	orgID := info.GetId()
	r.swAcc.PublishCreatedOrganization(&info)
	r.swAcc.QueueOrganizationSync(orgID)

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.CreateOrganizationResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:          orgID,
			DisplayName: info.GetDisplayName(),
			Role:        info.GetRole(),
		},
	}, nil
}

// queueOrgUpdateOp mounts the org SO and queues an UpdateOrgOp.
// Failures are logged as warnings since the cloud mutation already succeeded.
func (r *SpacewaveSessionResource) queueOrgUpdateOp(ctx context.Context, orgID string, op *s4wave_org.UpdateOrgOp) {
	// Prepare organization-specific logging before checking local authority.
	le := r.le.WithField("org-id", orgID)
	if !r.swAcc.HasCachedOwnerOrganization(orgID) {
		le.Debug("skipping local org update op: org SO is not owner-authoritative")
		return
	}
	if !r.swAcc.HasCachedSharedObject(orgID) {
		le.Debug("skipping local org update op: org SO not cached")
		return
	}
	ref := sobject.NewSharedObjectRef(
		r.swAcc.GetProviderID(),
		r.swAcc.GetAccountID(),
		orgID,
		provider_spacewave.SobjectBlockStoreID(orgID),
	)
	so, relSO, err := r.swAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Debug("failed to mount org SO for update op")
		return
	}
	defer relSO()

	// Encode the operation before queuing it on the organization SharedObject.
	opData, err := s4wave_org.MarshalUpdateOrgSOOp(op)
	if err != nil {
		le.WithError(err).Warn("failed to marshal update org op")
		return
	}
	if _, _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Debug("failed to queue update org op")
	}
}

// refreshOrganizationCaches invalidates cached org detail and optionally
// refreshes the org list cache after a successful cloud mutation.
func (r *SpacewaveSessionResource) refreshOrganizationCaches(
	ctx context.Context,
	orgID string,
	refreshList bool,
) {
	r.swAcc.InvalidateOrganizationState(orgID)
	if !refreshList {
		return
	}
	if err := r.swAcc.RefreshOrganizationList(ctx); err != nil {
		r.le.WithError(err).Warn("failed to refresh organization list")
	}
}

// WatchOrganizationState streams one organization's combined mutable state.
func (r *SpacewaveSessionResource) WatchOrganizationState(
	req *s4wave_provider_spacewave.WatchOrganizationStateRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOrganizationStateStream,
) error {
	// Require an organization ID before starting its state watch.
	orgID := req.GetOrgId()
	if orgID == "" {
		return errors.New("org_id is required")
	}

	// Watch the organization cache for state changes.
	ctx := strm.Context()
	var prev *s4wave_provider_spacewave.WatchOrganizationStateResponse
	for {
		// Take the wait channel before loading, so a change during the load
		// wakes the next iteration.
		ch := r.swAcc.ReadOrgState(nil)

		// Send the organization state when it differs from the last sent.
		resp, err := r.loadOrganizationState(ctx, orgID)
		if err != nil {
			return err
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		// Wait for the next organization change.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// loadOrganizationState loads a combined organization snapshot.
func (r *SpacewaveSessionResource) loadOrganizationState(
	ctx context.Context,
	orgID string,
) (*s4wave_provider_spacewave.WatchOrganizationStateResponse, error) {
	// Read one consistent organization snapshot for the response.
	info, inviteResp, roleID, err := r.swAcc.GetOrganizationSnapshot(ctx, orgID)
	if err != nil {
		return nil, err
	}
	members := make([]*s4wave_provider_spacewave.OrgMemberInfo, len(info.GetMembers()))
	for i, m := range info.GetMembers() {
		members[i] = &s4wave_provider_spacewave.OrgMemberInfo{
			Id:        m.GetId(),
			SubjectId: m.GetSubjectId(),
			RoleId:    m.GetRoleId(),
			CreatedAt: m.GetCreatedAt(),
			EntityId:  m.GetEntityId(),
		}
	}
	spaces := make([]*s4wave_provider_spacewave.OrgSpaceInfo, len(info.GetSpaces()))
	for i, s := range info.GetSpaces() {
		spaces[i] = &s4wave_provider_spacewave.OrgSpaceInfo{
			Id:          s.GetId(),
			DisplayName: s.GetDisplayName(),
			ObjectType:  s.GetObjectType(),
		}
	}

	// Project Cloud organization invites into the watch response.
	invites := make([]*s4wave_provider_spacewave.OrgInviteInfo, len(inviteResp.GetInvites()))
	for i, inv := range inviteResp.GetInvites() {
		invites[i] = &s4wave_provider_spacewave.OrgInviteInfo{
			Id:        inv.GetId(),
			Type:      inv.GetType(),
			Token:     inv.GetToken(),
			Uses:      inv.GetUses(),
			MaxUses:   inv.GetMaxUses(),
			ExpiresAt: inv.GetExpiresAt(),
		}
	}

	// Load the organization's root-state health when it has a root SharedObject.
	var rootState *s4wave_provider_spacewave.OrganizationRootStateInfo
	rootStateSOID := info.GetRootStateSoId()
	if rootStateSOID != "" {
		health, err := r.parent.loadSharedObjectHealthSnapshot(ctx, rootStateSOID)
		if err != nil {
			return nil, err
		}
		rootState = rootstate.BuildInfo(rootStateSOID, health, roleID)
	}

	return &s4wave_provider_spacewave.WatchOrganizationStateResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:               info.GetId(),
			DisplayName:      info.GetDisplayName(),
			Role:             roleID,
			BillingAccountId: info.GetBillingAccountId(),
		},
		Members:   members,
		Spaces:    spaces,
		Invites:   invites,
		RootState: rootState,
	}, nil
}

// DeleteOrganization deletes an organization.
func (r *SpacewaveSessionResource) DeleteOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.DeleteOrganizationRequest,
) (*s4wave_provider_spacewave.DeleteOrganizationResponse, error) {
	// Require an organization ID before deleting Cloud state.
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}

	// Delete the Cloud organization and refresh its cached state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.DeleteOrganization(ctx, orgID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)
	return &s4wave_provider_spacewave.DeleteOrganizationResponse{}, nil
}

// CreateOrgInvite creates an invite for an organization.
func (r *SpacewaveSessionResource) CreateOrgInvite(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateOrgInviteRequest,
) (*s4wave_provider_spacewave.CreateOrgInviteResponse, error) {
	// Create the Cloud invitation and decode its returned record.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	data, err := cli.CreateOrgInvite(ctx, req.GetOrgId(), req.GetType(), req.GetMaxUses(), req.GetExpiresAt(), req.GetEmail())
	if err != nil {
		return nil, err
	}
	var inv api.OrgInviteResponse
	if err := inv.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal invite")
	}
	if req.GetMaxUses() < 0 {
		return nil, errors.Errorf("invite max uses must not be negative: %d", req.GetMaxUses())
	}

	// Mirror the invite to the local org SO.
	var inviteType s4wave_org.OrgInviteType
	switch req.GetType() {
	case "code":
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_CODE
	case "email":
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_EMAIL
	default:
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_LINK
	}
	createInviteOp := &s4wave_org.CreateOrgInviteOp{
		Type:      inviteType,
		MaxUses:   uint32(req.GetMaxUses()), //nolint:gosec // MaxUses is a non-negative int32 and therefore fits uint32.
		Config:    req.GetEmail(),
		Timestamp: timestamppb.Now(),
	}
	if req.GetExpiresAt() > 0 {
		createInviteOp.ExpiresAt = timestamppb.New(time.UnixMilli(req.GetExpiresAt()))
	}
	r.refreshOrganizationCaches(ctx, req.GetOrgId(), false)
	r.queueOrgUpdateOp(ctx, req.GetOrgId(), &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_CreateInvite{
			CreateInvite: createInviteOp,
		},
	})

	// Map the Cloud invite record into the Session resource response.
	resp := &s4wave_provider_spacewave.CreateOrgInviteResponse{
		Invite: &s4wave_provider_spacewave.OrgInviteInfo{
			Id:        inv.GetId(),
			Type:      inv.GetType(),
			Token:     inv.GetToken(),
			Uses:      inv.GetUses(),
			MaxUses:   inv.GetMaxUses(),
			ExpiresAt: inv.GetExpiresAt(),
		},
	}
	return resp, nil
}

// JoinOrganization joins an organization via invite token.
func (r *SpacewaveSessionResource) JoinOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.JoinOrganizationRequest,
) (*s4wave_provider_spacewave.JoinOrganizationResponse, error) {
	// Join the Cloud organization and decode its returned state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	data, err := cli.JoinOrganization(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	var info api.OrgResponse
	if err := info.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal org")
	}

	// Refresh SO list so the org SO appears immediately for the joiner.
	if err := r.swAcc.RefreshSharedObjectList(ctx); err != nil {
		r.le.WithError(err).Warn("failed to refresh SO list after join")
	}
	r.refreshOrganizationCaches(ctx, info.GetId(), true)

	// Mirror the join to the local org SO.
	r.queueOrgUpdateOp(ctx, info.GetId(), &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_JoinViaInvite{
			JoinViaInvite: &s4wave_org.JoinOrgViaInviteOp{
				Token:     req.GetToken(),
				AccountId: r.swAcc.GetAccountID(),
				Timestamp: timestamppb.Now(),
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.JoinOrganizationResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:          info.GetId(),
			DisplayName: info.GetDisplayName(),
			Role:        info.GetRole(),
		},
	}, nil
}

// RevokeOrgInvite revokes an invite by ID.
func (r *SpacewaveSessionResource) RevokeOrgInvite(
	ctx context.Context,
	req *s4wave_provider_spacewave.RevokeOrgInviteRequest,
) (*s4wave_provider_spacewave.RevokeOrgInviteResponse, error) {
	// Require organization and invite IDs before revocation.
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	inviteID := req.GetInviteId()
	if inviteID == "" {
		return nil, errors.New("invite_id is required")
	}

	// Revoke the Cloud invite and refresh the cached organization state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.RevokeOrgInvite(ctx, orgID, inviteID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, false)

	// Mirror the successful invite revocation into the local organization SharedObject.
	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RevokeInvite{
			RevokeInvite: &s4wave_org.RevokeOrgInviteOp{
				InviteId: inviteID,
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.RevokeOrgInviteResponse{}, nil
}

// LeaveOrganization leaves an organization.
func (r *SpacewaveSessionResource) LeaveOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.LeaveOrganizationRequest,
) (*s4wave_provider_spacewave.LeaveOrganizationResponse, error) {
	// Require an organization ID before leaving its membership.
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}

	// Leave the Cloud organization and refresh its cached membership state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.LeaveOrganization(ctx, orgID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	// Mirror the successful membership departure into the local organization SharedObject.
	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RemoveMember{
			RemoveMember: &s4wave_org.RemoveOrgMember{
				AccountId: r.swAcc.GetAccountID(),
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.LeaveOrganizationResponse{}, nil
}

// RemoveOrgMember removes a member from an organization.
func (r *SpacewaveSessionResource) RemoveOrgMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.RemoveOrgMemberRequest,
) (*s4wave_provider_spacewave.RemoveOrgMemberResponse, error) {
	// Require organization and member IDs before removal.
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	memberID := req.GetMemberId()
	if memberID == "" {
		return nil, errors.New("member_id is required")
	}

	// Remove the Cloud organization member and refresh cached membership state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.RemoveOrgMember(ctx, orgID, memberID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	// Mirror the successful member removal into the local organization SharedObject.
	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RemoveMember{
			RemoveMember: &s4wave_org.RemoveOrgMember{
				AccountId: memberID,
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.RemoveOrgMemberResponse{}, nil
}

// UpdateOrganization updates an organization's display name.
func (r *SpacewaveSessionResource) UpdateOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.UpdateOrganizationRequest,
) (*s4wave_provider_spacewave.UpdateOrganizationResponse, error) {
	// Require an organization ID and display name before updating it.
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	displayName := req.GetDisplayName()
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}

	// Update the Cloud organization name and refresh its cached state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.UpdateOrganization(ctx, orgID, displayName); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	// Mirror the successful display-name change into the local organization SharedObject.
	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_UpdateDisplayName{
			UpdateDisplayName: &s4wave_org.UpdateOrgDisplayName{
				DisplayName: displayName,
			},
		},
	})

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.UpdateOrganizationResponse{}, nil
}
