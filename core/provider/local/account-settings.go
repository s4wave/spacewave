package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
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

	var ref *sobject.SharedObjectRef
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, SobjectBindingKey(accountSettingsBindingPurpose))
			if err != nil {
				return err
			}
			if !found || len(data) == 0 {
				return sobject.ErrSharedObjectNotFound
			}

			next := &sobject.SharedObjectRef{}
			if err := next.UnmarshalVT(data); err != nil {
				return err
			}
			if err := next.Validate(); err != nil {
				return err
			}
			provRef := next.GetProviderResourceRef()
			if provRef.GetProviderId() != a.t.accountInfo.GetProviderId() {
				return errors.New("account settings binding provider id mismatch")
			}
			if provRef.GetProviderAccountId() != a.t.accountInfo.GetProviderAccountId() {
				return errors.New("account settings binding account id mismatch")
			}
			if next.GetBlockStoreId() != SobjectBlockStoreID(provRef.GetId()) {
				return errors.New("account settings binding block store id mismatch")
			}
			ref = next
			return nil
		},
	)
	return ref, err
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

	data, err := ref.MarshalVT()
	if err != nil {
		return err
	}
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, SobjectBindingKey(accountSettingsBindingPurpose), data)
		},
	)
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
