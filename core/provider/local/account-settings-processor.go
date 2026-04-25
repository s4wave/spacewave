package provider_local

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// runAccountSettingsProcessor mounts the account settings SO and processes
// operations as the local validator. Blocks until ctx is canceled.
func (a *ProviderAccount) runAccountSettingsProcessor(ctx context.Context) error {
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}

	so, soRef, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer soRef()

	return so.ProcessOperations(ctx, true, account_settings.ProcessAccountSettingsOps)
}
