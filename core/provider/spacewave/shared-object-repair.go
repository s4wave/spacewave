package provider_spacewave

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// RepairSharedObject retries owner-side repair for a broken shared object.
func (a *ProviderAccount) RepairSharedObject(
	ctx context.Context,
	sharedObjectID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sharedObjectID == "" {
		return errors.New("shared object id is required")
	}
	if !a.canMutateCloudObjects() {
		return errors.New("mutation-capable cloud session is required")
	}

	cli, sessionPriv, _, err := a.getReadySessionClient(ctx)
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
	if isOrganizationRootSharedObject(meta, sharedObjectID) {
		if err := a.repairOrganizationRootSharedObject(
			ctx,
			cli,
			sessionPriv,
			sharedObjectID,
		); err != nil {
			return err
		}
		a.sobjects.RemoveKey(sharedObjectID)
		return nil
	}

	if err := a.repairStandaloneSharedObject(
		ctx,
		cli,
		sessionPriv,
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
	sessionPriv crypto.PrivKey,
	orgID string,
) error {
	// Repair a written root like any standalone object.
	state, _, _, err := cli.loadStandaloneConfigState(ctx, orgID)
	if err != nil {
		return err
	}
	root := state.GetRoot()
	if root != nil && root.GetInnerSeqno() != 0 {
		return a.repairStandaloneSharedObject(ctx, cli, sessionPriv, orgID)
	}

	// Reseed an empty root and restore the organization record.
	if err := a.reseedEmptySharedObject(ctx, cli, orgID); err != nil {
		return err
	}
	info, err := a.getOrganizationInfo(ctx, orgID)
	if err != nil {
		return err
	}
	return a.populateOrganizationSharedObject(ctx, orgID, info)
}

func (a *ProviderAccount) repairStandaloneSharedObject(
	ctx context.Context,
	cli *SessionClient,
	sessionPriv crypto.PrivKey,
	sharedObjectID string,
) error {
	// Reseed an object whose root was never written.
	state, _, _, err := cli.loadStandaloneConfigState(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	root := state.GetRoot()
	if root == nil || root.GetInnerSeqno() == 0 {
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

	// Decode the root state with the recovered grant.
	material, err := a.readSharedObjectRecoveryMaterial(
		ctx,
		cli,
		entityPriv,
		sharedObjectID,
	)
	if err != nil {
		return err
	}
	stateData, err := a.decodeSharedObjectRootStateData(
		sharedObjectID,
		root,
		material.GetGrantInner(),
	)
	if err != nil {
		return err
	}

	// Post the same state under the next root seqno.
	return a.postRepairedSharedObjectRoot(
		ctx,
		cli,
		sessionPriv,
		sharedObjectID,
		root.GetInnerSeqno()+1,
		material.GetGrantInner(),
		stateData,
	)
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

func (a *ProviderAccount) readSharedObjectRecoveryMaterial(
	ctx context.Context,
	cli *SessionClient,
	entityPriv crypto.PrivKey,
	sharedObjectID string,
) (*sobject.SOEntityRecoveryMaterial, error) {
	if cli == nil {
		return nil, errors.New("session client is required")
	}
	env, err := cli.GetSORecoveryEnvelope(ctx, sharedObjectID)
	if err != nil {
		return nil, err
	}
	if env.GetEntityId() != "" && env.GetEntityId() != a.accountID {
		return nil, sobject.ErrSharedObjectRecoveryEntityMismatch
	}
	material, err := sobject.UnlockSOEntityRecoveryEnvelope(
		[]crypto.PrivKey{entityPriv},
		env,
	)
	if err != nil {
		return nil, err
	}
	if material.GetEntityId() != "" && material.GetEntityId() != a.accountID {
		return nil, sobject.ErrSharedObjectRecoveryEntityMismatch
	}
	if material.GetGrantInner() == nil {
		return nil, errors.New("shared object recovery material is missing grant inner")
	}
	return material, nil
}

func (a *ProviderAccount) decodeSharedObjectRootStateData(
	sharedObjectID string,
	root *sobject.SORoot,
	grantInner *sobject.SOGrantInner,
) ([]byte, error) {
	if root == nil {
		return nil, errors.New("root is required")
	}
	if grantInner == nil {
		return nil, errors.New("grant inner is required")
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{
			Logger: a.le.WithField("sobject-id", sharedObjectID),
		},
		a.sfs,
		grantInner.GetTransformConf(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "build repair decode transformer")
	}
	innerDataDec, err := xfrm.DecodeBlock(root.GetInner())
	if err != nil {
		return nil, errors.Wrap(err, "decode repair root inner")
	}
	inner := &sobject.SORootInner{}
	if err := inner.UnmarshalVT(innerDataDec); err != nil {
		return nil, errors.Wrap(err, "unmarshal repair root inner")
	}
	if inner.GetSeqno() != root.GetInnerSeqno() {
		return nil, errors.Wrapf(
			sobject.ErrInvalidSeqno,
			"root had %d but inner had %d",
			root.GetInnerSeqno(),
			inner.GetSeqno(),
		)
	}
	return inner.GetStateData(), nil
}

func (a *ProviderAccount) postRepairedSharedObjectRoot(
	ctx context.Context,
	cli *SessionClient,
	sessionPriv crypto.PrivKey,
	sharedObjectID string,
	nextSeqno uint64,
	grantInner *sobject.SOGrantInner,
	stateData []byte,
) error {
	if grantInner == nil {
		return errors.New("grant inner is required")
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{
			Logger: a.le.WithField("sobject-id", sharedObjectID),
		},
		a.sfs,
		grantInner.GetTransformConf(),
	)
	if err != nil {
		return errors.Wrap(err, "build repair transformer")
	}
	innerDataDec, err := (&sobject.SORootInner{
		Seqno:     nextSeqno,
		StateData: stateData,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal repaired root inner")
	}
	innerDataEnc, err := xfrm.EncodeBlock(innerDataDec)
	if err != nil {
		return errors.Wrap(err, "encode repaired root inner")
	}

	root := &sobject.SORoot{
		InnerSeqno: nextSeqno,
		Inner:      innerDataEnc,
	}
	if err := root.SignInnerData(
		sessionPriv,
		sharedObjectID,
		root.GetInnerSeqno(),
		hash.RecommendedHashType,
	); err != nil {
		return errors.Wrap(err, "sign repaired root")
	}
	return cli.PostRoot(ctx, sharedObjectID, root, nil)
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
