package s4wave_session

import "context"

// WatchStorageBackends streams the account's storage backends.
func (s *Session) WatchStorageBackends(ctx context.Context) (SRPCSessionResourceService_WatchStorageBackendsClient, error) {
	return s.service.WatchStorageBackends(ctx, &WatchStorageBackendsRequest{})
}

// CheckStorageBackend runs the connectivity check on a saved or unsaved backend.
func (s *Session) CheckStorageBackend(ctx context.Context, req *CheckStorageBackendRequest) (*CheckStorageBackendResponse, error) {
	return s.service.CheckStorageBackend(ctx, req)
}

// AddStorageBackend saves a backend after its connectivity check passes.
func (s *Session) AddStorageBackend(ctx context.Context, req *AddStorageBackendRequest) (*AddStorageBackendResponse, error) {
	return s.service.AddStorageBackend(ctx, req)
}

// RemoveStorageBackend removes a backend that holds no Space.
func (s *Session) RemoveStorageBackend(ctx context.Context, storageBackendID string) error {
	_, err := s.service.RemoveStorageBackend(ctx, &RemoveStorageBackendRequest{StorageBackendId: storageBackendID})
	return err
}

// WatchSpaceStorage streams where a Space's blocks are stored and the
// progress of their upload.
func (s *Session) WatchSpaceStorage(ctx context.Context, sharedObjectID string) (SRPCSessionResourceService_WatchSpaceStorageClient, error) {
	return s.service.WatchSpaceStorage(ctx, &WatchSpaceStorageRequest{SharedObjectId: sharedObjectID})
}

// SetDefaultStorageBackend selects the backend new Spaces use, or the
// account's own storage when storageBackendID is empty.
func (s *Session) SetDefaultStorageBackend(ctx context.Context, storageBackendID string) error {
	_, err := s.service.SetDefaultStorageBackend(ctx, &SetDefaultStorageBackendRequest{StorageBackendId: storageBackendID})
	return err
}
