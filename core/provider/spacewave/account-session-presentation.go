package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// UpsertSessionPresentation mirrors private session presentation metadata into
// the account-settings shared object. Best-effort callers may log failures and
// continue because the live session set remains authoritative elsewhere.
func (a *ProviderAccount) UpsertSessionPresentation(
	ctx context.Context,
	peerID string,
	observed *api.ObservedSessionMetadata,
) error {
	if peerID == "" || observed == nil {
		return nil
	}
	if !a.canMutateCloudObjects() {
		return nil
	}

	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	return queueAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     peerID,
				Label:      observed.GetLabel(),
				DeviceType: observed.GetDeviceType(),
				ClientName: observed.GetClientName(),
				Os:         observed.GetOs(),
				Location:   observed.GetLocation(),
			},
		},
	})
}

// RemoveSessionPresentation removes mirrored private session presentation
// metadata from the account-settings shared object.
func (a *ProviderAccount) RemoveSessionPresentation(
	ctx context.Context,
	peerID string,
) error {
	if peerID == "" {
		return nil
	}
	if !a.canMutateCloudObjects() {
		return nil
	}

	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	return removeSessionPresentationFromSharedObject(ctx, so, peerID)
}

func removeSessionPresentationFromSharedObject(
	ctx context.Context,
	so sobject.SharedObject,
	peerID string,
) error {
	return queueAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveSessionPresentation{
			RemoveSessionPresentation: &account_settings.RemoveSessionPresentationOp{
				PeerId: peerID,
			},
		},
	})
}

func queueAccountSettingsOp(
	ctx context.Context,
	so sobject.SharedObject,
	op *account_settings.AccountSettingsOp,
) error {
	opData, err := op.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal account settings op")
	}
	if _, err := sobject.WriteOperation(ctx, so, opData, account_settings.ProcessAccountSettingsOps); err != nil {
		return errors.Wrap(err, "write account settings op")
	}
	return nil
}
