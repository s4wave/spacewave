package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
)

type organizationMemberNotFoundError struct {
	accountID string
	orgID     string
}

// Error names the account and the organization it is missing from.
func (e *organizationMemberNotFoundError) Error() string {
	return "account " + e.accountID + " is not a member of organization " + e.orgID
}

func isOrganizationMemberNotFound(err error) bool {
	var target *organizationMemberNotFoundError
	return errors.As(err, &target)
}

// reconcileOwnedOrganizationSpaces enrolls org members into spaces for which
// this account holds the organization owner role.
func (a *ProviderAccount) reconcileOwnedOrganizationSpaces(ctx context.Context, orgID string) error {
	// Only a session that may change Cloud objects reconciles.
	if !a.canMutateCloudObjects() {
		return nil
	}

	// Read the organizations this account belongs to.
	orgs, err := a.getOrganizationList(ctx)
	if err != nil {
		return err
	}

	// Reconcile each organization this account owns, or only orgID when set.
	for _, org := range orgs {
		if orgID != "" && org.GetId() != orgID {
			continue
		}
		if !isOrganizationOwnerRole(org.GetRole()) {
			continue
		}
		if err := a.reconcileOrganization(ctx, org.GetId()); err != nil {
			return errors.Wrapf(err, "reconcile organization %s", org.GetId())
		}
	}

	return nil
}

// reconcilePendingParticipant enrolls a single account into a shared object
// after the cloud worker notifies the authorized organization session.
func (a *ProviderAccount) reconcilePendingParticipant(ctx context.Context, soID, accountID string) error {
	// Require the shared object, the account and a session that may enroll.
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if accountID == "" {
		return errors.New("account id is required")
	}
	if !a.canMutateCloudObjects() {
		return nil
	}
	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

	// Resolve the organization that owns the shared object.
	orgID, ok := a.GetCachedSharedObjectOrganizationID(soID)
	if !ok {
		meta, err := a.GetSharedObjectMetadata(ctx, soID)
		if err != nil {
			return errors.Wrap(err, "get shared object metadata")
		}
		if meta.GetOwnerType() != sobject.OwnerTypeOrganization {
			return nil
		}
		orgID = meta.GetOwnerId()
	}
	if orgID == "" {
		return nil
	}

	// Read the member's role, refreshing the organization once on a miss.
	role, err := a.getOrganizationMemberRole(ctx, orgID, accountID)
	if err != nil {
		if !isOrganizationMemberNotFound(err) {
			return err
		}

		a.fetchAndUpdateOrgList(ctx)
		if recErr := a.reconcileOwnedOrganizationSpaces(ctx, orgID); recErr != nil {
			return errors.Wrapf(
				recErr,
				"reconcile organization %s after pending participant miss",
				orgID,
			)
		}

		role, err = a.getOrganizationMemberRole(ctx, orgID, accountID)
		if err != nil {
			if isOrganizationMemberNotFound(err) {
				return nil
			}
			return err
		}
	}

	// Enroll the member into the mounted shared object.
	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	return a.enrollMountedSpaceMember(ctx, cli, swSO, soID, accountID, role)
}

// reconcileMemberSession enrolls a member's sessions into a shared object
// after a member_session_added notification for sessionPeerID. The Cloud sends
// one notification for every session of the member, so a notification whose
// session is already enrolled returns without a Cloud request, and one
// enroll-member request adds each new session.
func (a *ProviderAccount) reconcileMemberSession(ctx context.Context, soID, accountID, sessionPeerID string) error {
	// Require the shared object, the member and a session that may enroll.
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if accountID == "" {
		return errors.New("account id is required")
	}
	if !a.canMutateCloudObjects() {
		return nil
	}
	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

	// Mount the shared object and read its state.
	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}

	// Skip a session that is already a participant with a current grant.
	if participantConfigForPeer(state.GetConfig(), sessionPeerID) != nil &&
		peerEnrolledInCurrentEpoch(swSO.host.GetKeyEpochs(), sessionPeerID) {
		return nil
	}

	// Enroll at the member's current role, or the local peer's higher role
	// when the member is this account.
	role := participantRoleForAccount(
		state.GetConfig(),
		accountID,
		sobject.SOParticipantRole_SOParticipantRole_WRITER,
	)
	if accountID == a.GetAccountID() {
		role = max(role, state.GetConfig().GetParticipantRole(swSO.GetPeerID().String()))
	}
	return a.enrollMountedSpaceMember(ctx, cli, swSO, soID, accountID, role)
}

// revokeMemberSession removes a participant from a shared object after a
// member_session_removed notification. Mounts the SO and calls
// RemoveSOParticipant with SESSION_REVOKED revocation reason.
func (a *ProviderAccount) revokeMemberSession(ctx context.Context, soID, sessionPeerID string) error {
	// Require the shared object, the session peer and a session that may
	// change Cloud objects.
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if sessionPeerID == "" {
		return errors.New("session peer id is required")
	}
	if !a.canMutateCloudObjects() {
		return nil
	}

	// Mount the shared object.
	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	// Remove the session's peer with the session revoked reason.
	revInfo := &sobject.SORevocationInfo{
		Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_SESSION_REVOKED,
	}
	_, err = swSO.RemoveParticipantWithRevocation(ctx, sessionPeerID, revInfo)
	return err
}

// getOrganizationList returns the cached org list when available, otherwise it
// fetches it from the cloud.
func (a *ProviderAccount) getOrganizationList(ctx context.Context) ([]*api.OrgResponse, error) {
	// Use the cached list when it is valid.
	var orgs []*api.OrgResponse
	var valid bool
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		orgs = append(orgs, a.orgList...)
		valid = a.orgListValid
	})
	if valid {
		return orgs, nil
	}

	// Otherwise fetch it from the Cloud.
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	// List the organizations.
	data, err := cli.ListOrganizations(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list organizations")
	}

	// Decode the list.
	resp := &api.ListOrgsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal organization list")
	}
	return resp.GetOrganizations(), nil
}

// reconcileOrganization enrolls members for every space owned by a single
// organization.
func (a *ProviderAccount) reconcileOrganization(ctx context.Context, orgID string) error {
	// Read the organization's members and spaces.
	info, err := a.getOrganizationInfo(ctx, orgID)
	if err != nil {
		return err
	}

	// Map the members to roles; with none there is nothing to enroll.
	memberRoles := buildOrganizationMemberRoleMap(info)
	if len(memberRoles) == 0 {
		return nil
	}

	// Require a session client for the enrollments.
	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

	// Reconcile each space with its own mount.
	for _, space := range info.GetSpaces() {
		spaceID := space.GetId()
		if spaceID == "" {
			continue
		}

		swSO, rel, err := a.mountSpaceSO(ctx, spaceID)
		if err != nil {
			return errors.Wrapf(err, "mount space %s", spaceID)
		}

		err = a.reconcileMountedOrganizationSpace(
			ctx,
			cli,
			swSO,
			spaceID,
			memberRoles,
		)
		rel()
		if err != nil {
			return errors.Wrapf(err, "reconcile mounted space %s", spaceID)
		}
	}

	return nil
}

// getOrganizationInfo retrieves full org info including members and spaces.
func (a *ProviderAccount) getOrganizationInfo(ctx context.Context, orgID string) (*api.GetOrgResponse, error) {
	// Require a session client.
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	// Fetch the organization.
	data, err := cli.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, errors.Wrapf(err, "get organization %s", orgID)
	}

	// Decode it.
	info := &api.GetOrgResponse{}
	if err := info.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal organization info")
	}
	return info, nil
}

// getOrganizationMemberRole returns the shared object role implied by an org
// member binding.
func (a *ProviderAccount) getOrganizationMemberRole(ctx context.Context, orgID, accountID string) (sobject.SOParticipantRole, error) {
	info, err := a.getOrganizationInfo(ctx, orgID)
	if err != nil {
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, err
	}

	for _, member := range info.GetMembers() {
		if member.GetSubjectId() != accountID {
			continue
		}

		role, ok := organizationMemberRoleToSOParticipantRole(member.GetRoleId())
		if !ok {
			return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, errors.Errorf(
				"unsupported organization role %q",
				member.GetRoleId(),
			)
		}
		return role, nil
	}

	return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, &organizationMemberNotFoundError{
		accountID: accountID,
		orgID:     orgID,
	}
}

// reconcileMountedOrganizationSpace enrolls all org members into an already
// mounted space shared object.
func (a *ProviderAccount) reconcileMountedOrganizationSpace(
	ctx context.Context,
	cli *SessionClient,
	swSO *SharedObject,
	spaceID string,
	memberRoles map[string]sobject.SOParticipantRole,
) error {
	// Read the current state, before the enrollments below change it.
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}

	// Enroll every member but this account.
	for accountID, role := range memberRoles {
		if accountID == a.GetAccountID() {
			continue
		}
		if err := a.enrollMountedSpaceMember(ctx, cli, swSO, spaceID, accountID, role); err != nil {
			return errors.Wrapf(err, "enroll %s", accountID)
		}
	}

	// Remove every peer of a departed member in one config change.
	var stale []string
	for _, p := range state.GetConfig().GetParticipants() {
		accountID := p.GetEntityId()
		if accountID == "" || accountID == a.GetAccountID() {
			continue
		}
		if _, ok := memberRoles[accountID]; ok {
			continue
		}
		stale = append(stale, p.GetPeerId())
	}
	if len(stale) == 0 {
		return nil
	}
	revInfo := &sobject.SORevocationInfo{
		Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_ORG_REMOVED,
	}
	if _, err := swSO.RemoveParticipantsWithRevocation(ctx, stale, revInfo); err != nil {
		return errors.Wrapf(err, "remove stale participants %v", stale)
	}
	return nil
}

// enrollMountedSpaceMember resolves the target account's current session peers
// and adds them to a mounted shared object.
func (a *ProviderAccount) enrollMountedSpaceMember(
	ctx context.Context,
	cli *SessionClient,
	swSO *SharedObject,
	spaceID string,
	accountID string,
	role sobject.SOParticipantRole,
) error {
	enrollResp, err := cli.EnrollMember(ctx, spaceID, accountID, false)
	if err != nil {
		return errors.Wrap(err, "resolve member peers")
	}

	for _, p := range enrollResp.GetPeers() {
		targetPub, err := session.ExtractPublicKeyFromPeerID(p.GetPeerId())
		if err != nil {
			return errors.Wrapf(err, "extract pubkey for %s", p.GetPeerId())
		}

		if _, _, _, err := swSO.AddParticipant(ctx, p.GetPeerId(), targetPub, role, accountID, enrollResp.GetEntityId()); err != nil {
			return errors.Wrapf(err, "add participant %s", p.GetPeerId())
		}
	}

	return nil
}

// mountSpaceSO mounts a cloud shared object by ID and returns the typed value.
func (a *ProviderAccount) mountSpaceSO(
	ctx context.Context,
	spaceID string,
) (*SharedObject, func(), error) {
	// Mount the shared object through its reference.
	ref := sobject.NewSharedObjectRef(
		a.GetProviderID(),
		a.GetAccountID(),
		spaceID,
		SobjectBlockStoreID(spaceID),
	)
	so, rel, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	// Accept only a Spacewave shared object.
	swSO, ok := so.(*SharedObject)
	if !ok {
		rel()
		return nil, nil, errors.New("unexpected shared object type")
	}
	return swSO, rel, nil
}

// buildOrganizationMemberRoleMap maps org member account IDs to shared object
// participant roles.
func buildOrganizationMemberRoleMap(
	info *api.GetOrgResponse,
) map[string]sobject.SOParticipantRole {
	memberRoles := make(map[string]sobject.SOParticipantRole)
	for _, member := range info.GetMembers() {
		role, ok := organizationMemberRoleToSOParticipantRole(member.GetRoleId())
		if !ok {
			continue
		}
		memberRoles[member.GetSubjectId()] = role
	}
	return memberRoles
}

// isOrganizationOwnerRole returns true when the role identifies an org owner.
func isOrganizationOwnerRole(roleID string) bool {
	return roleID == "owner" || roleID == "org:owner"
}

// organizationMemberRoleToSOParticipantRole maps org membership roles to shared
// object participant roles.
func organizationMemberRoleToSOParticipantRole(
	roleID string,
) (sobject.SOParticipantRole, bool) {
	if roleID == "owner" || roleID == "org:owner" {
		return sobject.SOParticipantRole_SOParticipantRole_OWNER, true
	}
	if roleID == "member" || roleID == "org:member" {
		return sobject.SOParticipantRole_SOParticipantRole_WRITER, true
	}
	return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, false
}

// OrganizationMemberRoleToSOParticipantRole maps org membership roles to
// shared object participant roles.
func OrganizationMemberRoleToSOParticipantRole(
	roleID string,
) (sobject.SOParticipantRole, bool) {
	return organizationMemberRoleToSOParticipantRole(roleID)
}

// participantRoleForAccount returns the highest role any peer of accountID
// holds in cfg, or fallback when it is higher or the account has none.
func participantRoleForAccount(
	cfg *sobject.SharedObjectConfig,
	accountID string,
	fallback sobject.SOParticipantRole,
) sobject.SOParticipantRole {
	if cfg == nil || accountID == "" {
		return fallback
	}

	role := fallback
	for _, participant := range cfg.GetParticipants() {
		if participant.GetEntityId() != accountID {
			continue
		}
		if participant.GetRole() > role {
			role = participant.GetRole()
		}
	}
	return role
}
