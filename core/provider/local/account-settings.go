package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
)

// accountSettingsBindingPurpose identifies the local account-settings binding.
const accountSettingsBindingPurpose = "account-settings"

// GetAccountSettingsRef returns the bound account settings SharedObjectRef.
func (a *ProviderAccount) GetAccountSettingsRef(ctx context.Context) (*sobject.SharedObjectRef, error) {
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, SobjectBindingKey(accountSettingsBindingPurpose))
	if err != nil {
		return nil, err
	}
	if !found || len(data) == 0 {
		return nil, sobject.ErrSharedObjectNotFound
	}

	ref := &sobject.SharedObjectRef{}
	if err := ref.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	provRef := ref.GetProviderResourceRef()
	if provRef.GetProviderId() != a.t.accountInfo.GetProviderId() {
		return nil, errors.New("account settings binding provider id mismatch")
	}
	if provRef.GetProviderAccountId() != a.t.accountInfo.GetProviderAccountId() {
		return nil, errors.New("account settings binding account id mismatch")
	}
	if ref.GetBlockStoreId() != SobjectBlockStoreID(provRef.GetId()) {
		return nil, errors.New("account settings binding block store id mismatch")
	}

	return ref, nil
}

func (a *ProviderAccount) writeAccountSettingsRef(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
) error {
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer release()

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	data, err := ref.MarshalVT()
	if err != nil {
		return err
	}
	if err := otx.Set(ctx, SobjectBindingKey(accountSettingsBindingPurpose), data); err != nil {
		return err
	}

	return otx.Commit(ctx)
}

// EnsureAccountSettingsSO returns the bound account settings SharedObjectRef,
// creating and binding a unique-id local settings SO when absent.
func (a *ProviderAccount) EnsureAccountSettingsSO(ctx context.Context) (*sobject.SharedObjectRef, error) {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relMtx()

	ref, err := a.GetAccountSettingsRef(ctx)
	if err == nil {
		return ref, nil
	}
	if err != sobject.ErrSharedObjectNotFound {
		return nil, err
	}

	meta := account_settings.NewSharedObjectMeta()
	for {
		ref, err = a.createSharedObjectLocked(ctx, ulid.NewULID(), meta)
		if err == sobject.ErrSharedObjectExists {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := a.writeAccountSettingsRef(ctx, ref); err != nil {
			if delErr := a.deleteSharedObjectLocked(ctx, ref.GetProviderResourceRef().GetId()); delErr != nil {
				a.le.WithError(delErr).Warn("failed to clean up unbound account settings shared object")
			}
			return nil, err
		}
		return ref, nil
	}
}
