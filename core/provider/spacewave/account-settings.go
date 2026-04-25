package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// ensureAccountSettingsSharedObject ensures the account settings shared object
// exists in the cloud for this account.
func (a *ProviderAccount) ensureAccountSettingsSharedObject(
	ctx context.Context,
) (*sobject.SharedObjectRef, error) {
	if ref := a.getAccountSettingsBindingRefSnapshot(); ref != nil {
		return ref, nil
	}

	binding, err := a.sessionClient.EnsureAccountSObjectBinding(
		ctx,
		account_settings.BindingPurpose,
	)
	if err != nil {
		return nil, err
	}
	ref := a.buildSharedObjectRef(binding.GetSoId())
	if binding.GetState() == api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY {
		return ref, nil
	}

	_, err = a.CreateSharedObject(
		ctx,
		binding.GetSoId(),
		account_settings.NewSharedObjectMeta(),
		"",
		"",
	)
	if err != nil {
		var ce *cloudError
		if !errors.As(err, &ce) || ce.StatusCode != 409 {
			return nil, err
		}
	}

	if _, err := a.sessionClient.FinalizeAccountSObjectBinding(
		ctx,
		account_settings.BindingPurpose,
		binding.GetSoId(),
	); err != nil {
		return nil, err
	}
	a.BumpLocalEpoch()
	return ref, nil
}

// GetAccountSettingsRef returns the bound cloud account settings SharedObjectRef.
func (a *ProviderAccount) GetAccountSettingsRef(
	ctx context.Context,
) (*sobject.SharedObjectRef, error) {
	return a.ensureAccountSettingsSharedObject(ctx)
}

func (a *ProviderAccount) getAccountSettingsBindingRefSnapshot() *sobject.SharedObjectRef {
	var ref *sobject.SharedObjectRef
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		info := a.state.info
		if info == nil {
			return
		}
		for _, binding := range info.GetAccountSobjectBindings() {
			if binding.GetPurpose() != account_settings.BindingPurpose {
				continue
			}
			if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY {
				return
			}
			ref = a.buildSharedObjectRef(binding.GetSoId())
			return
		}
	})
	return ref
}
