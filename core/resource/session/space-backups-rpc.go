package resource_session

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// SetSpaceBackup configures a backup only on the connected daemon's device.
func (r *SessionResource) SetSpaceBackup(ctx context.Context, request *s4wave_session.SetSpaceBackupRequest) (*s4wave_session.SetSpaceBackupResponse, error) {
	// Forward to the stable account-owned routine registry.
	account, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	backup, err := account.SetSpaceBackup(ctx, request.GetBackup())
	if err != nil {
		return nil, err
	}
	return &s4wave_session.SetSpaceBackupResponse{Backup: backup}, nil
}

// RunSpaceBackup streams the requested account-owned pass until it settles.
func (r *SessionResource) RunSpaceBackup(request *s4wave_session.RunSpaceBackupRequest, stream s4wave_session.SRPCSessionResourceService_RunSpaceBackupStream) error {
	// The stream observes the pass; its disconnection releases only the watch.
	account, err := r.localProviderAccount()
	if err != nil {
		return err
	}
	return account.RunSpaceBackup(stream.Context(), request.GetBackupId(), func(state *account_settings.SpaceBackupState) error {
		return stream.Send(state)
	})
}

// WatchSpaceBackups watches this device's configuration and progress only.
func (r *SessionResource) WatchSpaceBackups(_ *s4wave_session.WatchSpaceBackupsRequest, stream s4wave_session.SRPCSessionResourceService_WatchSpaceBackupsStream) error {
	// No storage credential or snapshot decryption material enters this stream.
	account, err := r.localProviderAccount()
	if err != nil {
		return err
	}
	return account.WatchSpaceBackups(stream.Context(), func(state *account_settings.SpaceBackupList) error {
		return stream.Send(state)
	})
}
