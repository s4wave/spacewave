package s4wave_session

import "context"

// SetSpaceBackup saves a device-local backup configuration.
func (s *Session) SetSpaceBackup(ctx context.Context, request *SetSpaceBackupRequest) (*SetSpaceBackupResponse, error) {
	return s.service.SetSpaceBackup(ctx, request)
}

// RunSpaceBackup starts a pass and streams its state until it settles.
func (s *Session) RunSpaceBackup(ctx context.Context, request *RunSpaceBackupRequest) (SRPCSessionResourceService_RunSpaceBackupClient, error) {
	return s.service.RunSpaceBackup(ctx, request)
}

// WatchSpaceBackups sends the connected device's complete registry first,
// followed by changed entries keyed by backup ID. Reopening resets the view.
func (s *Session) WatchSpaceBackups(ctx context.Context) (SRPCSessionResourceService_WatchSpaceBackupsClient, error) {
	return s.service.WatchSpaceBackups(ctx, &WatchSpaceBackupsRequest{})
}
