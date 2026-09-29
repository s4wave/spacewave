package resource_session

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// errStorageBackendsLocalOnly is returned when a non-local account manages
// storage backends.
var errStorageBackendsLocalOnly = errors.New("storage backends require a local provider account")

// WatchStorageBackends streams the account's storage backends and the Spaces
// placed on each whenever the account settings change.
func (r *SessionResource) WatchStorageBackends(
	req *s4wave_session.WatchStorageBackendsRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchStorageBackendsStream,
) error {
	// Tie the stream's resources to its context.
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	// Mount the account settings shared object and access its state.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return err
	}
	soRef, err := localAcc.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, mountRef, err := sobject.ExMountSharedObject(ctx, r.b, soRef, false, ctxCancel)
	if err != nil {
		return err
	}
	defer mountRef.Release()

	// Access the settings object's state container.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	// Stream a response whenever the settings change materially.
	var snap sobject.SharedObjectStateSnapshot
	var prev *s4wave_session.WatchStorageBackendsResponse
	for {
		snap, err = stateCtr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		settings, err := decodeAccountSettings(ctx, snap)
		if err != nil {
			return err
		}
		resp := buildStorageBackendsResponse(settings)
		if prev != nil && resp.EqualVT(prev) {
			continue
		}
		if err := strm.Send(resp); err != nil {
			return err
		}
		prev = resp
	}
}

// buildStorageBackendsResponse lists each backend with its placed Spaces.
func buildStorageBackendsResponse(settings *account_settings.AccountSettings) *s4wave_session.WatchStorageBackendsResponse {
	// List each backend with the Spaces placed on it.
	backends := make([]*s4wave_session.StorageBackendInfo, 0, len(settings.GetStorageBackends()))
	for _, backend := range settings.GetStorageBackends() {
		info := &s4wave_session.StorageBackendInfo{Backend: backend}
		for _, blockStoreID := range settings.PlacedBlockStoreIDs(backend.GetId()) {
			spaceID, name := settings.FindSpaceByBlockStore(blockStoreID)
			if spaceID == "" {
				spaceID = blockStoreID
			}
			info.PlacedSpaces = append(info.PlacedSpaces, &s4wave_session.PlacedSpace{SpaceId: spaceID, Name: name})
		}
		backends = append(backends, info)
	}

	// Return the backend list with the default selection.
	return &s4wave_session.WatchStorageBackendsResponse{
		StorageBackends:         backends,
		DefaultStorageBackendId: settings.GetDefaultStorageBackendId(),
	}
}

// CheckStorageBackend runs the connectivity check against a saved backend,
// or against an unsaved location and credentials.
func (r *SessionResource) CheckStorageBackend(
	ctx context.Context,
	req *s4wave_session.CheckStorageBackendRequest,
) (*s4wave_session.CheckStorageBackendResponse, error) {
	// Load the local provider account.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}

	// Check a saved backend by id.
	if id := req.GetStorageBackendId(); id != "" {
		result, err := localAcc.CheckStorageBackend(ctx, id)
		if err != nil {
			return nil, err
		}
		return &s4wave_session.CheckStorageBackendResponse{Result: result}, nil
	}

	// Check an unsaved S3 location and credentials.
	if err := req.GetS3().Validate(); err != nil {
		return nil, err
	}
	result := provider_local.CheckS3Location(ctx, req.GetS3(), req.GetCredentials())
	return &s4wave_session.CheckStorageBackendResponse{Result: result}, nil
}

// AddStorageBackend saves a backend after its connectivity check passes.
func (r *SessionResource) AddStorageBackend(
	ctx context.Context,
	req *s4wave_session.AddStorageBackendRequest,
) (*s4wave_session.AddStorageBackendResponse, error) {
	// Save the backend after the account's connectivity check.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	id, check, err := localAcc.AddStorageBackend(ctx, req.GetDisplayName(), req.GetS3(), req.GetCredentials(), req.GetSetDefault())
	if err != nil {
		return nil, err
	}

	// Return the saved backend id and its check result.
	return &s4wave_session.AddStorageBackendResponse{StorageBackendId: id, Check: check}, nil
}

// RemoveStorageBackend removes a backend that holds no Space.
func (r *SessionResource) RemoveStorageBackend(
	ctx context.Context,
	req *s4wave_session.RemoveStorageBackendRequest,
) (*s4wave_session.RemoveStorageBackendResponse, error) {
	// Remove the unused backend from the account.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	if err := localAcc.RemoveStorageBackend(ctx, req.GetStorageBackendId()); err != nil {
		return nil, err
	}

	// Acknowledge the removal.
	return &s4wave_session.RemoveStorageBackendResponse{}, nil
}

// SetDefaultStorageBackend selects the backend new Spaces use.
func (r *SessionResource) SetDefaultStorageBackend(
	ctx context.Context,
	req *s4wave_session.SetDefaultStorageBackendRequest,
) (*s4wave_session.SetDefaultStorageBackendResponse, error) {
	// Select the default backend on the account.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	if err := localAcc.SetDefaultStorageBackend(ctx, req.GetStorageBackendId()); err != nil {
		return nil, err
	}

	// Acknowledge the selection.
	return &s4wave_session.SetDefaultStorageBackendResponse{}, nil
}

// localProviderAccount returns the session's local provider account.
func (r *SessionResource) localProviderAccount() (*provider_local.ProviderAccount, error) {
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok || localAcc == nil {
		return nil, errStorageBackendsLocalOnly
	}
	return localAcc, nil
}

// decodeAccountSettings decodes the account settings in a snapshot.
func decodeAccountSettings(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
) (*account_settings.AccountSettings, error) {
	// Read the snapshot's root inner state.
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}

	// Decode the account settings payload when present.
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return settings, nil
}

// placeNewSpaceBlockStore places a new Space's block store on the requested
// or default storage backend. Returns the placed block store id, or empty when
// the Space stays on the account's own storage.
func (r *SessionResource) placeNewSpaceBlockStore(
	ctx context.Context,
	soID string,
	req *s4wave_session.CreateSpaceRequest,
) (string, error) {
	// Load the session's provider account.
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok || localAcc == nil {
		// Reject an explicit backend request on a non-local account.
		if req.GetStorageBackendId() != "" {
			return "", errStorageBackendsLocalOnly
		}
		return "", nil
	}

	// Resolve the target backend for the new Space.
	backendID, err := localAcc.ResolveNewSpaceStorageBackend(ctx, req.GetStorageBackendId(), req.GetAccountStorage())
	if err != nil || backendID == "" {
		return "", err
	}

	// Place the Space's block store on the resolved backend.
	blockStoreID := provider_local.SobjectBlockStoreID(soID)
	if err := localAcc.PlaceBlockStore(ctx, blockStoreID, backendID); err != nil {
		return "", err
	}
	return blockStoreID, nil
}

// releaseNewSpaceBlockStore removes the placement of a Space that failed to
// create. The placement names no data, so a failure only logs.
func (r *SessionResource) releaseNewSpaceBlockStore(ctx context.Context, blockStoreID string) {
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return
	}
	if err := localAcc.PlaceBlockStore(context.WithoutCancel(ctx), blockStoreID, ""); err != nil {
		r.le.WithError(err).Warn("unable to remove placement of uncreated space")
	}
}

// WatchSpaceStorage streams where a Space's blocks are stored and the
// progress of their upload.
func (r *SessionResource) WatchSpaceStorage(
	req *s4wave_session.WatchSpaceStorageRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchSpaceStorageStream,
) error {
	// Stream the Space's upload status until the stream ends.
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return err
	}
	var prev *s4wave_session.WatchSpaceStorageResponse
	return localAcc.WatchUploadStatus(strm.Context(), req.GetSharedObjectId(), func(status provider_local.UploadStatus) error {
		// Build the response from the upload status.
		resp := &s4wave_session.WatchSpaceStorageResponse{
			StorageBackendId:   status.Backend.GetId(),
			StorageBackendName: status.Backend.GetDisplayName(),
			PendingBlocks:      int64(status.Pending),
			PendingBytes:       status.PendingBytes,
		}
		if status.Err != nil {
			resp.UploadError = status.Err.Error()
		}

		// Send the response when it differs from the previous one.
		if prev != nil && resp.EqualVT(prev) {
			return nil
		}
		prev = resp
		return strm.Send(resp)
	})
}

// MoveSpaceStorage moves a Space's blocks to a storage backend or to the
// account's own storage, and streams the progress until the destination holds
// every block.
func (r *SessionResource) MoveSpaceStorage(
	req *s4wave_session.MoveSpaceStorageRequest,
	strm s4wave_session.SRPCSessionResourceService_MoveSpaceStorageStream,
) error {
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return err
	}
	return localAcc.MoveSpaceStorage(
		strm.Context(),
		req.GetSharedObjectId(),
		req.GetStorageBackendId(),
		func(progress provider_local.MoveProgress) error {
			resp := &s4wave_session.MoveSpaceStorageResponse{
				Phase:         moveSpaceStoragePhase(progress.Phase),
				BlocksFetched: int64(progress.Fetched),
				BlocksTotal:   int64(progress.Total),
				PendingBlocks: int64(progress.Upload.Pending),
				PendingBytes:  progress.Upload.PendingBytes,
			}
			if progress.Upload.Err != nil {
				resp.UploadError = progress.Upload.Err.Error()
			}
			return strm.Send(resp)
		},
	)
}

// moveSpaceStoragePhase converts a move phase to its wire value.
func moveSpaceStoragePhase(phase provider_local.MovePhase) s4wave_session.MoveSpaceStoragePhase {
	switch phase {
	case provider_local.MovePhaseFetch:
		return s4wave_session.MoveSpaceStoragePhase_MoveSpaceStoragePhase_FETCH
	case provider_local.MovePhaseUpload:
		return s4wave_session.MoveSpaceStoragePhase_MoveSpaceStoragePhase_UPLOAD
	case provider_local.MovePhaseDone:
		return s4wave_session.MoveSpaceStoragePhase_MoveSpaceStoragePhase_DONE
	default:
		return s4wave_session.MoveSpaceStoragePhase_MoveSpaceStoragePhase_UNKNOWN
	}
}
