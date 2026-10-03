package provider_spacewave

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// RepairSharedObject retries owner-side repair for a broken shared object.
func (a *ProviderAccount) RepairSharedObject(
	ctx context.Context,
	sharedObjectID string,
) error {
	// Require a live context, an object, and a mutation-capable session.
	if err := ctx.Err(); err != nil {
		return err
	}
	if sharedObjectID == "" {
		return errors.New("shared object id is required")
	}
	if !a.canMutateCloudObjects() {
		return errors.New("mutation-capable cloud session is required")
	}

	// Reach the cloud and authorize the mutation.
	cli, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}
	meta, err := a.GetSharedObjectMetadata(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	if err := a.authorizeSharedObjectMutation(ctx, meta, sharedObjectID); err != nil {
		return err
	}

	// An organization root has its own repair.
	if isOrganizationRootSharedObject(meta, sharedObjectID) {
		if err := a.repairOrganizationRootSharedObject(
			ctx,
			cli,
			sharedObjectID,
		); err != nil {
			return err
		}
		a.sobjects.RemoveKey(sharedObjectID)
		return nil
	}

	// Repair a standalone object and drop its cached mount.
	if err := a.repairStandaloneSharedObject(
		ctx,
		cli,
		sharedObjectID,
	); err != nil {
		return err
	}
	a.sobjects.RemoveKey(sharedObjectID)
	return nil
}

// ReinitializeSharedObject destructively rewrites a broken shared object in place.
func (a *ProviderAccount) ReinitializeSharedObject(
	ctx context.Context,
	sharedObjectID string,
) error {
	// Require a live context, an object and a mutation-capable session.
	if err := ctx.Err(); err != nil {
		return err
	}
	if sharedObjectID == "" {
		return errors.New("shared object id is required")
	}
	if !a.canMutateCloudObjects() {
		return errors.New("mutation-capable cloud session is required")
	}
	cli, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}

	// Require that this account may rewrite the object.
	meta, err := a.GetSharedObjectMetadata(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	if err := a.authorizeSharedObjectMutation(ctx, meta, sharedObjectID); err != nil {
		return err
	}

	// Clear the object on the Cloud and write a fresh genesis.
	if err := cli.ReinitializeSharedObject(ctx, sharedObjectID); err != nil {
		return err
	}
	if err := a.reseedEmptySharedObject(ctx, cli, sharedObjectID); err != nil {
		return err
	}

	// Restore the organization record in an organization root.
	if isOrganizationRootSharedObject(meta, sharedObjectID) {
		info, err := a.getOrganizationInfo(ctx, sharedObjectID)
		if err != nil {
			return err
		}
		if err := a.populateOrganizationSharedObject(ctx, sharedObjectID, info); err != nil {
			return err
		}
	}

	// Drop the tracker so the next mount reads the new state.
	a.sobjects.RemoveKey(sharedObjectID)
	return nil
}

// reseedEmptySharedObject clears the local recovery state of a shared object
// and writes a fresh standalone genesis owned by this account.
func (a *ProviderAccount) reseedEmptySharedObject(
	ctx context.Context,
	cli *SessionClient,
	sharedObjectID string,
) error {
	// Drop the verified chain and recovery state the new genesis replaces.
	if err := a.clearSharedObjectRecoveryLocalState(ctx, sharedObjectID); err != nil {
		return err
	}

	// Write a genesis that records this account's username.
	username, err := a.getUsername(ctx)
	if err != nil {
		return err
	}
	le := a.le.WithField("sobject-id", sharedObjectID)
	_, err = cli.InitEmptyStandaloneSpace(ctx, le, a.accountID, username, sharedObjectID)
	return err
}

func isOrganizationRootSharedObject(
	meta *api.SpaceMetadataResponse,
	sharedObjectID string,
) bool {
	if meta == nil {
		return false
	}
	return meta.GetObjectType() == "organization" &&
		meta.GetOwnerType() == sobject.OwnerTypeOrganization &&
		meta.GetOwnerId() == sharedObjectID
}

func (a *ProviderAccount) authorizeSharedObjectMutation(
	ctx context.Context,
	meta *api.SpaceMetadataResponse,
	sharedObjectID string,
) error {
	if meta == nil {
		return errors.New("shared object metadata is required")
	}
	switch meta.GetOwnerType() {
	case sobject.OwnerTypeAccount:
		if meta.GetOwnerId() != a.accountID {
			return errors.New("only the shared object owner can repair or reinitialize this shared object")
		}
		return nil
	case sobject.OwnerTypeOrganization:
		orgID := meta.GetOwnerId()
		if orgID == "" {
			return errors.Errorf(
				"organization-owned shared object %s is missing owner id",
				sharedObjectID,
			)
		}
		_, _, roleID, err := a.GetOrganizationSnapshot(ctx, orgID)
		if err != nil {
			return err
		}
		if roleID != "owner" && roleID != "org:owner" {
			return errors.New("only organization owners can repair or reinitialize this shared object")
		}
		return nil
	default:
		return errors.Errorf(
			"shared object %s has unsupported owner type %q",
			sharedObjectID,
			meta.GetOwnerType(),
		)
	}
}

func (a *ProviderAccount) repairOrganizationRootSharedObject(
	ctx context.Context,
	cli *SessionClient,
	orgID string,
) error {
	// Repair a written organization like any standalone object.
	state, _, _, err := cli.loadStandaloneConfigState(ctx, orgID)
	if err != nil {
		return err
	}
	if state.GetCheckpoint() != nil {
		return a.repairStandaloneSharedObject(ctx, cli, orgID)
	}

	// Reseed an empty object and restore the organization record.
	if err := a.reseedEmptySharedObject(ctx, cli, orgID); err != nil {
		return err
	}
	info, err := a.getOrganizationInfo(ctx, orgID)
	if err != nil {
		return err
	}
	return a.populateOrganizationSharedObject(ctx, orgID, info)
}

// repairStandaloneSharedObject reseeds an object that was never written, and
// otherwise enrolls this session as a participant again with an unlocked
// entity key. The held checkpoint stays valid, so nothing is re-signed.
func (a *ProviderAccount) repairStandaloneSharedObject(
	ctx context.Context,
	cli *SessionClient,
	sharedObjectID string,
) error {
	// Reseed an object that was never written.
	state, _, _, err := cli.loadStandaloneConfigState(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	if state.GetCheckpoint() == nil {
		return a.reseedEmptySharedObject(ctx, cli, sharedObjectID)
	}

	// Enroll this session with an unlocked entity key.
	store := a.getEntityKeyStore()
	if store == nil {
		return sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	entityPriv, _, ok := store.GetAnyUnlockedKey()
	if !ok {
		return sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	if _, err := cli.SelfEnrollSpacePeer(
		ctx,
		entityPriv,
		a.accountID,
		sharedObjectID,
	); err != nil && !errors.Is(err, sobject.ErrNotParticipant) {
		return err
	}
	return nil
}

func (a *ProviderAccount) clearSharedObjectRecoveryLocalState(
	ctx context.Context,
	sharedObjectID string,
) error {
	if err := a.InvalidateVerifiedChain(ctx, sharedObjectID); err != nil {
		return err
	}
	a.getWriteTicketOwner(sharedObjectID).Invalidate()
	return nil
}

func (a *ProviderAccount) populateOrganizationSharedObject(
	ctx context.Context,
	orgID string,
	info *api.GetOrgResponse,
) error {
	if info == nil {
		return errors.New("organization info is required")
	}
	so, relSO, err := a.mountSpaceSO(ctx, orgID)
	if err != nil {
		return err
	}
	defer relSO()

	creatorID := a.accountID
	for _, member := range info.GetMembers() {
		if member.GetRoleId() == "org:owner" {
			creatorID = member.GetSubjectId()
			break
		}
	}

	initOp := &s4wave_org.InitOrganizationOp{
		OrgObjectKey:     s4wave_org.OrgObjectKey,
		DisplayName:      info.GetDisplayName(),
		CreatorAccountId: creatorID,
		Timestamp:        timestamppb.Now(),
	}
	initData, err := s4wave_org.MarshalInitOrgSOOp(initOp)
	if err != nil {
		return errors.Wrap(err, "marshal init org op")
	}
	if _, err := so.QueueOperation(ctx, initData); err != nil {
		return errors.Wrap(err, "queue init org op")
	}

	for _, member := range info.GetMembers() {
		if member.GetSubjectId() == creatorID {
			continue
		}
		role := s4wave_org.OrgRoleMember
		if member.GetRoleId() == "org:owner" {
			role = s4wave_org.OrgRoleOwner
		}
		joinedAt := timestamppb.Now()
		if member.GetCreatedAt() > 0 {
			joinedAt = timestamppb.New(time.UnixMilli(member.GetCreatedAt()))
		}
		op := &s4wave_org.UpdateOrgOp{
			OrgObjectKey: s4wave_org.OrgObjectKey,
			Body: &s4wave_org.UpdateOrgOp_AddMember{
				AddMember: &s4wave_org.AddOrgMember{
					Member: &s4wave_org.OrgMemberInfo{
						AccountId:   member.GetSubjectId(),
						DisplayRole: role,
						JoinedAt:    joinedAt,
					},
				},
			},
		}
		opData, err := s4wave_org.MarshalUpdateOrgSOOp(op)
		if err != nil {
			return errors.Wrap(err, "marshal add member op")
		}
		if _, err := so.QueueOperation(ctx, opData); err != nil {
			return errors.Wrap(err, "queue add member op")
		}
	}

	for _, space := range info.GetSpaces() {
		op := &s4wave_org.UpdateOrgOp{
			OrgObjectKey: s4wave_org.OrgObjectKey,
			Body: &s4wave_org.UpdateOrgOp_AddChildSo{
				AddChildSo: &s4wave_org.AddOrgChildSo{
					SharedObjectId: space.GetId(),
				},
			},
		}
		opData, err := s4wave_org.MarshalUpdateOrgSOOp(op)
		if err != nil {
			return errors.Wrap(err, "marshal add child shared object op")
		}
		if _, err := so.QueueOperation(ctx, opData); err != nil {
			return errors.Wrap(err, "queue add child shared object op")
		}
	}

	return nil
}
