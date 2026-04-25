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

func (e *organizationMemberNotFoundError) Error() string {
	return "account " + e.accountID + " is not a member of organization " + e.orgID
}

func isOrganizationMemberNotFound(err error) bool {
	var target *organizationMemberNotFoundError
	return errors.As(err, &target)
}

// reconcileOwnedOrganizationSpaces enrolls org members into spaces owned by
// organizations this account owns.
func (a *ProviderAccount) reconcileOwnedOrganizationSpaces(ctx context.Context, orgID string) error {
	if !a.canMutateCloudObjects() {
		return nil
	}

	orgs, err := a.getOrganizationList(ctx)
	if err != nil {
		return err
	}

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
// after the cloud worker notifies the owner session.
func (a *ProviderAccount) reconcilePendingParticipant(ctx context.Context, soID, accountID string) error {
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

	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	return a.enrollMountedSpaceMember(ctx, cli, swSO, soID, accountID, role)
}

// reconcileMemberSession enrolls a member's sessions into a shared object
// after a member_session_added notification. Mounts the SO and calls
// enrollMountedSpaceMember to add the member's current session peers.
func (a *ProviderAccount) reconcileMemberSession(ctx context.Context, soID, accountID string) error {
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

	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}
	role := participantRoleForAccount(
		state.GetConfig(),
		accountID,
		sobject.SOParticipantRole_SOParticipantRole_WRITER,
	)
	if accountID == a.GetAccountID() {
		localRole := participantRoleForPeer(
			state.GetConfig(),
			swSO.GetPeerID().String(),
			sobject.SOParticipantRole_SOParticipantRole_UNKNOWN,
		)
		if localRole > role {
			role = localRole
		}
	}
	return a.enrollMountedSpaceMember(ctx, cli, swSO, soID, accountID, role)
}

// revokeMemberSession removes a participant from a shared object after a
// member_session_removed notification. Mounts the SO and calls
// RemoveSOParticipant with SESSION_REVOKED revocation reason.
func (a *ProviderAccount) revokeMemberSession(ctx context.Context, soID, sessionPeerID string) error {
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if sessionPeerID == "" {
		return errors.New("session peer id is required")
	}
	if !a.canMutateCloudObjects() {
		return nil
	}

	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	revInfo := &sobject.SORevocationInfo{
		Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_SESSION_REVOKED,
	}
	_, err = swSO.RemoveParticipantWithRevocation(ctx, sessionPeerID, revInfo)
	return err
}

// getOrganizationList returns the cached org list when available, otherwise it
// fetches it from the cloud.
func (a *ProviderAccount) getOrganizationList(ctx context.Context) ([]*api.OrgResponse, error) {
	var orgs []*api.OrgResponse
	var valid bool
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		orgs = append(orgs, a.orgList...)
		valid = a.orgListValid
	})
	if valid {
		return orgs, nil
	}

	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	data, err := cli.ListOrganizations(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list organizations")
	}

	resp := &api.ListOrgsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal organization list")
	}
	return resp.GetOrganizations(), nil
}

// reconcileOrganization enrolls members for every space owned by a single
// organization.
func (a *ProviderAccount) reconcileOrganization(ctx context.Context, orgID string) error {
	info, err := a.getOrganizationInfo(ctx, orgID)
	if err != nil {
		return err
	}

	memberRoles := buildOrganizationMemberRoleMap(info)
	if len(memberRoles) == 0 {
		return nil
	}

	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

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
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	data, err := cli.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, errors.Wrapf(err, "get organization %s", orgID)
	}

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
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}

	for accountID, role := range memberRoles {
		if accountID == a.GetAccountID() {
			continue
		}
		if err := a.enrollMountedSpaceMember(ctx, cli, swSO, spaceID, accountID, role); err != nil {
			return errors.Wrapf(err, "enroll %s", accountID)
		}
	}

	revInfo := &sobject.SORevocationInfo{
		Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_ORG_REMOVED,
	}
	for _, p := range state.GetConfig().GetParticipants() {
		accountID := p.GetEntityId()
		if accountID == "" || accountID == a.GetAccountID() {
			continue
		}
		if _, ok := memberRoles[accountID]; ok {
			continue
		}
		if _, err := swSO.RemoveParticipantWithRevocation(ctx, p.GetPeerId(), revInfo); err != nil {
			return errors.Wrapf(err, "remove stale participant %s", p.GetPeerId())
		}
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

		if _, err := swSO.AddParticipant(ctx, p.GetPeerId(), targetPub, role, accountID); err != nil {
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

func participantRoleForPeer(
	cfg *sobject.SharedObjectConfig,
	peerID string,
	fallback sobject.SOParticipantRole,
) sobject.SOParticipantRole {
	if cfg == nil || peerID == "" {
		return fallback
	}

	for _, participant := range cfg.GetParticipants() {
		if participant.GetPeerId() == peerID {
			return participant.GetRole()
		}
	}
	return fallback
}
