package provider_spacewave

import (
	"context"
	"net/http"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// bootstrapOrgSharedObjects creates org SOs for owner-visible orgs in the
// cloud org list that do not yet have a local SharedObject. Non-owner member
// accounts intentionally skip bootstrap because the local org SO mirror is not
// authoritative for them yet and attempting to seed it can mount an org SO the
// session peer is not allowed to validate.
//
// Detection consults the cloud-side organizations.root_state_so_id back-
// reference (returned via GetOrgResponse.RootStateSoId): the org has a root SO
// when that field is non-empty AND the local SO list contains it. Empty means
// the cloud has not yet seeded the org root SO and we should create it.
func (a *ProviderAccount) bootstrapOrgSharedObjects(ctx context.Context) {
	if !a.canMutateCloudObjects() {
		return
	}

	// Read current owner org list.
	var orgIDs []string
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.orgListValid {
			return
		}
		for _, org := range a.orgList {
			if !isOrganizationOwnerRole(org.GetRole()) {
				continue
			}
			orgIDs = append(orgIDs, org.GetId())
		}
	})
	if len(orgIDs) == 0 {
		return
	}

	// Read current SO list. Index by SO id for membership checks.
	soList := a.soListCtr.GetValue()
	if soList == nil {
		return
	}
	existingSOs := make(map[string]bool)
	for _, entry := range soList.GetSharedObjects() {
		existingSOs[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}

	// Create missing org SOs with full cloud state. The cloud-side
	// organizations.root_state_so_id back-reference is the source of truth
	// for whether the org has a seeded root SO; the local SO list is only
	// used to skip when that root is already mirrored locally.
	for _, orgID := range orgIDs {
		info, err := a.getOrganizationInfo(ctx, orgID)
		if err != nil {
			a.le.WithField("org-id", orgID).WithError(err).Debug("bootstrap: failed to fetch org info")
			continue
		}
		rootSO := info.GetRootStateSoId()
		if rootSO != "" && existingSOs[rootSO] {
			continue
		}
		a.createOrgSOForBootstrap(ctx, orgID, info)
	}
}

// createOrgSOForBootstrap fetches full org state from the cloud (when the
// caller did not already pass it) and creates a local org SO seeded with the
// real members, spaces, and creator.
func (a *ProviderAccount) createOrgSOForBootstrap(ctx context.Context, orgID string, info *api.GetOrgResponse) {
	le := a.le.WithField("org-id", orgID)

	if info == nil {
		var err error
		info, err = a.getOrganizationInfo(ctx, orgID)
		if err != nil {
			le.WithError(err).Debug("bootstrap: failed to fetch org info")
			return
		}
	}

	// Determine the creator (org:owner member) for InitOrganizationOp.
	creatorID := a.accountID
	for _, m := range info.GetMembers() {
		if m.GetRoleId() == "org:owner" {
			creatorID = m.GetSubjectId()
			break
		}
	}

	ref, err := a.CreateSharedObject(ctx, orgID, s4wave_org.NewOrgSharedObjectMeta(info.GetDisplayName()), sobject.OwnerTypeOrganization, orgID)
	if err != nil {
		var ce *cloudError
		if errors.As(err, &ce) && ce.StatusCode == http.StatusConflict {
			le.WithError(err).Debug("bootstrap: org SO already exists")
			return
		}
		le.WithError(err).Debug("bootstrap: failed to create org SO")
		return
	}

	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Debug("bootstrap: failed to mount org SO")
		return
	}
	defer relSO()

	// Queue InitOrganizationOp with the real creator.
	initOp := &s4wave_org.InitOrganizationOp{
		OrgObjectKey:     s4wave_org.OrgObjectKey,
		DisplayName:      info.GetDisplayName(),
		CreatorAccountId: creatorID,
		Timestamp:        timestamppb.Now(),
	}
	opData, err := s4wave_org.MarshalInitOrgSOOp(initOp)
	if err != nil {
		le.WithError(err).Warn("bootstrap: failed to marshal init org op")
		return
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Debug("bootstrap: failed to queue init org op")
		return
	}

	// Seed additional members (InitOrganizationOp already adds the creator).
	for _, m := range info.GetMembers() {
		if m.GetSubjectId() == creatorID {
			continue
		}
		role := s4wave_org.OrgRoleMember
		if m.GetRoleId() == "org:owner" {
			role = s4wave_org.OrgRoleOwner
		}
		joinedAt := timestamppb.Now()
		if m.GetCreatedAt() > 0 {
			joinedAt = timestamppb.New(time.UnixMilli(m.GetCreatedAt()))
		}
		addOp := &s4wave_org.UpdateOrgOp{
			OrgObjectKey: s4wave_org.OrgObjectKey,
			Body: &s4wave_org.UpdateOrgOp_AddMember{
				AddMember: &s4wave_org.AddOrgMember{
					Member: &s4wave_org.OrgMemberInfo{
						AccountId:   m.GetSubjectId(),
						DisplayRole: role,
						JoinedAt:    joinedAt,
					},
				},
			},
		}
		addData, err := s4wave_org.MarshalUpdateOrgSOOp(addOp)
		if err != nil {
			le.WithError(err).Warn("bootstrap: failed to marshal add member op")
			continue
		}
		if _, err := so.QueueOperation(ctx, addData); err != nil {
			le.WithError(err).Debug("bootstrap: failed to queue add member op")
		}
	}

	// Seed child SO references (spaces).
	for _, space := range info.GetSpaces() {
		childOp := &s4wave_org.UpdateOrgOp{
			OrgObjectKey: s4wave_org.OrgObjectKey,
			Body: &s4wave_org.UpdateOrgOp_AddChildSo{
				AddChildSo: &s4wave_org.AddOrgChildSo{
					SharedObjectId: space.GetId(),
				},
			},
		}
		childData, err := s4wave_org.MarshalUpdateOrgSOOp(childOp)
		if err != nil {
			le.WithError(err).Warn("bootstrap: failed to marshal add child SO op")
			continue
		}
		if _, err := so.QueueOperation(ctx, childData); err != nil {
			le.WithError(err).Debug("bootstrap: failed to queue add child SO op")
		}
	}
}
