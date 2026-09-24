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
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

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
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

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
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	if id := req.GetStorageBackendId(); id != "" {
		result, err := localAcc.CheckStorageBackend(ctx, id)
		if err != nil {
			return nil, err
		}
		return &s4wave_session.CheckStorageBackendResponse{Result: result}, nil
	}
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
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	id, check, err := localAcc.AddStorageBackend(ctx, req.GetDisplayName(), req.GetS3(), req.GetCredentials(), req.GetSetDefault())
	if err != nil {
		return nil, err
	}
	return &s4wave_session.AddStorageBackendResponse{StorageBackendId: id, Check: check}, nil
}

// RemoveStorageBackend removes a backend that holds no Space.
func (r *SessionResource) RemoveStorageBackend(
	ctx context.Context,
	req *s4wave_session.RemoveStorageBackendRequest,
) (*s4wave_session.RemoveStorageBackendResponse, error) {
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	if err := localAcc.RemoveStorageBackend(ctx, req.GetStorageBackendId()); err != nil {
		return nil, err
	}
	return &s4wave_session.RemoveStorageBackendResponse{}, nil
}

// SetDefaultStorageBackend selects the backend new Spaces use.
func (r *SessionResource) SetDefaultStorageBackend(
	ctx context.Context,
	req *s4wave_session.SetDefaultStorageBackendRequest,
) (*s4wave_session.SetDefaultStorageBackendResponse, error) {
	localAcc, err := r.localProviderAccount()
	if err != nil {
		return nil, err
	}
	if err := localAcc.SetDefaultStorageBackend(ctx, req.GetStorageBackendId()); err != nil {
		return nil, err
	}
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
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return settings, nil
}
