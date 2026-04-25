package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// sharedObjectMetadataStatus describes the cache state for one shared object.
type sharedObjectMetadataStatus uint8

const (
	// sharedObjectMetadataInvalid means the entry is missing or must be seeded.
	sharedObjectMetadataInvalid sharedObjectMetadataStatus = iota
	// sharedObjectMetadataValid means metadata contains a usable snapshot.
	sharedObjectMetadataValid
	// sharedObjectMetadataDeleted means the shared object is known deleted.
	sharedObjectMetadataDeleted
)

// sharedObjectMetadataState stores cached metadata for one shared object.
type sharedObjectMetadataState struct {
	// metadata is the cached full metadata snapshot for one shared object.
	metadata *api.SpaceMetadataResponse
	// status indicates whether metadata is valid, invalid, or deleted.
	status sharedObjectMetadataStatus
	// seed coordinates concurrent callers around a single seed HTTP fetch.
	// Guarded by accountBcast like the rest of the state.
	seed providerSeed
}

// GetSharedObjectMetadata returns full shared-object metadata from the account cache.
func (a *ProviderAccount) GetSharedObjectMetadata(
	ctx context.Context,
	soID string,
) (*api.SpaceMetadataResponse, error) {
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}
	if metadata, status := a.getSharedObjectMetadataSnapshot(soID); status == sharedObjectMetadataValid {
		return metadata, nil
	} else if status == sharedObjectMetadataDeleted {
		return nil, ErrSharedObjectMetadataDeleted
	}

	var seed *providerSeed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = &a.getOrCreateSharedObjectMetadataStateLocked(soID).seed
	})

	if err := seed.Run(ctx, &a.accountBcast, func(ctx context.Context) error {
		return a.syncSharedObjectMetadata(ctx, soID)
	}); err != nil {
		return nil, err
	}

	metadata, status := a.getSharedObjectMetadataSnapshot(soID)
	if status == sharedObjectMetadataValid {
		return metadata, nil
	}
	if status == sharedObjectMetadataDeleted {
		return nil, ErrSharedObjectMetadataDeleted
	}
	return nil, errors.New("shared object metadata seed produced no metadata")
}

// getSharedObjectMetadataSnapshot returns the cached metadata snapshot for an SO.
func (a *ProviderAccount) getSharedObjectMetadataSnapshot(
	soID string,
) (*api.SpaceMetadataResponse, sharedObjectMetadataStatus) {
	var metadata *api.SpaceMetadataResponse
	var status sharedObjectMetadataStatus
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.sharedObjectMetadata == nil {
			return
		}
		state := a.state.sharedObjectMetadata[soID]
		if state == nil {
			return
		}
		status = state.status
		metadata = cloneSharedObjectMetadata(state.metadata)
	})
	return metadata, status
}

// syncSharedObjectMetadata fetches and stores full metadata for one shared object.
func (a *ProviderAccount) syncSharedObjectMetadata(
	ctx context.Context,
	soID string,
) error {
	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not ready")
	}

	data, err := cli.GetSOMetadata(ctx, soID)
	if err != nil {
		return errors.Wrap(err, "get shared object metadata")
	}
	metadata := &api.SpaceMetadataResponse{}
	if err := metadata.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal shared object metadata")
	}
	a.SetSharedObjectMetadata(soID, metadata)
	return nil
}

// UpdateSharedObjectMetadata updates cloud metadata and stores the returned snapshot.
func (a *ProviderAccount) UpdateSharedObjectMetadata(
	ctx context.Context,
	soID string,
	metadata *api.SpaceMetadataResponse,
) (*api.SpaceMetadataResponse, error) {
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not ready")
	}
	data, err := cli.UpdateSOMetadata(ctx, soID, metadata)
	if err != nil {
		return nil, errors.Wrap(err, "update shared object metadata")
	}
	next := &api.SpaceMetadataResponse{}
	if err := next.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal shared object metadata")
	}
	a.SetSharedObjectMetadata(soID, next)
	a.PatchSharedObjectListMetadata(soID, next)
	return next.CloneVT(), nil
}

// SetSharedObjectMetadata stores a valid metadata snapshot for one shared object.
func (a *ProviderAccount) SetSharedObjectMetadata(
	soID string,
	metadata *api.SpaceMetadataResponse,
) {
	if soID == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSharedObjectMetadataStateLocked(soID)
		next := cloneSharedObjectMetadata(metadata)
		if state.status == sharedObjectMetadataValid && sharedObjectMetadataEqual(state.metadata, next) {
			return
		}
		state.metadata = next
		state.status = sharedObjectMetadataValid
		broadcast()
	})
}

// InvalidateSharedObjectMetadata marks one shared-object metadata cache entry stale.
func (a *ProviderAccount) InvalidateSharedObjectMetadata(
	soID string,
) {
	if soID == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.sharedObjectMetadata == nil {
			return
		}
		state := a.state.sharedObjectMetadata[soID]
		if state == nil || state.status == sharedObjectMetadataInvalid {
			return
		}
		state.status = sharedObjectMetadataInvalid
		broadcast()
	})
}

// InvalidateSharedObjectMetadataCache marks cached metadata entries stale.
func (a *ProviderAccount) InvalidateSharedObjectMetadataCache() {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if len(a.state.sharedObjectMetadata) == 0 {
			return
		}
		changed := false
		for _, state := range a.state.sharedObjectMetadata {
			if state == nil || state.status == sharedObjectMetadataInvalid ||
				state.status == sharedObjectMetadataDeleted {
				continue
			}
			state.status = sharedObjectMetadataInvalid
			changed = true
		}
		if changed {
			broadcast()
		}
	})
}

// DeleteSharedObjectMetadata marks one shared-object metadata cache entry deleted.
func (a *ProviderAccount) DeleteSharedObjectMetadata(
	soID string,
) {
	if soID == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSharedObjectMetadataStateLocked(soID)
		if state.status == sharedObjectMetadataDeleted && state.metadata == nil {
			return
		}
		state.metadata = nil
		state.status = sharedObjectMetadataDeleted
		broadcast()
	})
}

// getOrCreateSharedObjectMetadataStateLocked returns metadata cache state for an SO.
func (a *ProviderAccount) getOrCreateSharedObjectMetadataStateLocked(
	soID string,
) *sharedObjectMetadataState {
	if a.state.sharedObjectMetadata == nil {
		a.state.sharedObjectMetadata = make(map[string]*sharedObjectMetadataState)
	}
	state := a.state.sharedObjectMetadata[soID]
	if state == nil {
		state = &sharedObjectMetadataState{}
		a.state.sharedObjectMetadata[soID] = state
	}
	return state
}

// cloneSharedObjectMetadata clones one cached metadata snapshot.
func cloneSharedObjectMetadata(
	metadata *api.SpaceMetadataResponse,
) *api.SpaceMetadataResponse {
	if metadata == nil {
		return nil
	}
	return metadata.CloneVT()
}

// sharedObjectMetadataEqual compares two cached metadata snapshots.
func sharedObjectMetadataEqual(
	a *api.SpaceMetadataResponse,
	b *api.SpaceMetadataResponse,
) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualVT(b)
}
