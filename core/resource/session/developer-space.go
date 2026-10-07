package resource_session

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// developerSpaceName is the name of the Space created as an account's
// developer Space.
const developerSpaceName = "Developer"

// accountSettingsProvider is a provider account that keeps AccountSettings.
type accountSettingsProvider interface {
	// GetAccountSettingsRef returns the bound account settings SharedObjectRef.
	GetAccountSettingsRef(ctx context.Context) (*sobject.SharedObjectRef, error)
}

// EnsureDeveloperSpace returns the account's developer Space. The first call
// on an account creates the Space and records it in the account settings, so
// every device of the account finds the same one. When the recorded Space is
// gone, it creates a replacement and records it in place of the missing one.
func (r *SessionResource) EnsureDeveloperSpace(
	ctx context.Context,
	req *s4wave_session.EnsureDeveloperSpaceRequest,
) (*s4wave_session.EnsureDeveloperSpaceResponse, error) {
	// Mount the account settings.
	providerAcc := r.session.GetProviderAccount()
	settingsAcc, ok := providerAcc.(accountSettingsProvider)
	if !ok {
		return nil, errors.New("provider account has no account settings")
	}
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}
	settingsRef, err := settingsAcc.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	settingsSO, release, err := soFeature.MountSharedObject(ctx, settingsRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer release()

	// Reuse the developer Space the account already has.
	settings, err := readAccountSettings(ctx, settingsSO)
	if err != nil {
		return nil, err
	}
	previousID := settings.GetDeveloperSpaceId()
	if previousID != "" {
		exists, err := spaceExists(ctx, soFeature, previousID)
		if err != nil {
			return nil, err
		}
		if exists {
			return &s4wave_session.EnsureDeveloperSpaceResponse{SharedObjectId: previousID}, nil
		}
	}

	// Create the Space and record it in place of the one observed above.
	soRef, _, err := r.createSpace(ctx, &s4wave_session.CreateSpaceRequest{SpaceName: developerSpaceName})
	if err != nil {
		return nil, err
	}
	spaceID := soRef.GetProviderResourceRef().GetId()
	opData, err := (&account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_SetDeveloperSpace{
			SetDeveloperSpace: &account_settings.SetDeveloperSpaceOp{
				SpaceId:         spaceID,
				PreviousSpaceId: previousID,
			},
		},
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal account settings op")
	}
	_, err = sobject.WriteOperation(ctx, settingsSO, opData, account_settings.ProcessAccountSettingsOps)
	if err == nil {
		return &s4wave_session.EnsureDeveloperSpaceResponse{SharedObjectId: spaceID}, nil
	}

	// Another device replaced the observed Space first: use its Space and
	// delete ours. Keep ours when settings cannot be read, since the write may
	// have landed.
	settings, readErr := readAccountSettings(ctx, settingsSO)
	if readErr != nil || settings.GetDeveloperSpaceId() == spaceID {
		return nil, err
	}
	if delErr := soFeature.DeleteSharedObject(context.WithoutCancel(ctx), spaceID); delErr != nil {
		r.le.WithError(delErr).Warn("unable to delete developer space that lost the race")
	}
	if winner := settings.GetDeveloperSpaceId(); winner != "" && winner != previousID {
		return &s4wave_session.EnsureDeveloperSpaceResponse{SharedObjectId: winner}, nil
	}
	return nil, err
}

// readAccountSettings returns the current account settings of so.
func readAccountSettings(ctx context.Context, so sobject.SharedObject) (*account_settings.AccountSettings, error) {
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	return account_settings.ReadSnapshot(ctx, snap)
}

// spaceExists reports whether the account lists the SharedObject id.
func spaceExists(ctx context.Context, soFeature sobject.SharedObjectProvider, id string) (bool, error) {
	soListCtr, release, err := soFeature.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return false, err
	}
	defer release()
	entry, err := lookupSharedObjectListEntry(ctx, soFeature, soListCtr, id)
	return entry != nil, err
}
