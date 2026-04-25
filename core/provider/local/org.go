package provider_local

import (
	"context"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	"github.com/sirupsen/logrus"
)

// CreateOrgSharedObject creates an organization SharedObject and queues InitOrganizationOp.
// Used by the local provider to create orgs without any cloud API call.
func (a *ProviderAccount) CreateOrgSharedObject(ctx context.Context, orgID, displayName string) error {
	le := a.le.WithField("org-id", orgID)

	ref, err := a.CreateSharedObject(ctx, orgID, s4wave_org.NewOrgSharedObjectMeta(displayName), sobject.OwnerTypeOrganization, orgID)
	if err != nil {
		return err
	}

	return a.initOrgSO(ctx, le, ref.GetProviderResourceRef().GetId(), displayName)
}

// initOrgSO mounts an org SO and queues InitOrganizationOp.
func (a *ProviderAccount) initOrgSO(ctx context.Context, le *logrus.Entry, soID, displayName string) error {
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	blockStoreID := SobjectBlockStoreID(soID)

	ref := sobject.NewSharedObjectRef(providerID, accountID, soID, blockStoreID)
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Warn("failed to mount org SO")
		return err
	}
	defer relSO()

	initOp := &s4wave_org.InitOrganizationOp{
		OrgObjectKey:     s4wave_org.OrgObjectKey,
		DisplayName:      displayName,
		CreatorAccountId: accountID,
		Timestamp:        timestamppb.Now(),
	}
	opData, err := s4wave_org.MarshalInitOrgSOOp(initOp)
	if err != nil {
		le.WithError(err).Warn("failed to marshal init org op")
		return err
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Warn("failed to queue init org op")
		return err
	}

	return nil
}

// QueueOrgUpdateOp queues an UpdateOrgOp on the org SO.
func (a *ProviderAccount) QueueOrgUpdateOp(ctx context.Context, orgID string, op *s4wave_org.UpdateOrgOp) error {
	le := a.le.WithField("org-id", orgID)
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	blockStoreID := SobjectBlockStoreID(orgID)

	ref := sobject.NewSharedObjectRef(providerID, accountID, orgID, blockStoreID)
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Debug("failed to mount org SO for update op")
		return err
	}
	defer relSO()

	opData, err := s4wave_org.MarshalUpdateOrgSOOp(op)
	if err != nil {
		le.WithError(err).Warn("failed to marshal update org op")
		return err
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Debug("failed to queue update org op")
		return err
	}

	return nil
}
