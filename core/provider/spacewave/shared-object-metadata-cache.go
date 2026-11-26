package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/seedflight"
	"github.com/s4wave/spacewave/core/sobject"
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
	// gen increments on every store, invalidation, or deletion so an
	// in-flight fetch can detect that it was superseded.
	gen uint64
	// seed coordinates concurrent callers around a single seed HTTP fetch.
	// Guarded by accountBcast like the rest of the state.
	seed seedflight.Seed
}

// GetSharedObjectMetadata returns full shared-object metadata from the account cache.
func (a *ProviderAccount) GetSharedObjectMetadata(
	ctx context.Context,
	soID string,
) (*api.SpaceMetadataResponse, error) {
	// Serve a settled cache entry directly.
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}
	if metadata, status := a.getSharedObjectMetadataSnapshot(soID); status == sharedObjectMetadataValid {
		return metadata, nil
	} else if status == sharedObjectMetadataDeleted {
		return nil, ErrSharedObjectMetadataDeleted
	}

	// Otherwise join or start the entry's seed fetch.
	var seed *seedflight.Seed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = &a.getOrCreateSharedObjectMetadataStateLocked(soID).seed
	})
	if err := seed.RunWhenReady(
		ctx,
		&a.accountBcast,
		func() bool {
			state := a.state.sharedObjectMetadata[soID]
			return state != nil && state.status != sharedObjectMetadataInvalid
		},
		func(ctx context.Context) error {
			return a.syncSharedObjectMetadata(ctx, soID)
		},
	); err != nil {
		return nil, err
	}

	// Return the snapshot the seed settled.
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
		// Copy the entry, if any, so callers cannot mutate the cache.
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
//
// A fetch superseded by a store or deletion is discarded. A fetch superseded
// by an invalidation is discarded and repeated.
func (a *ProviderAccount) syncSharedObjectMetadata(
	ctx context.Context,
	soID string,
) error {
	cli, err := a.GetSessionClient()
	if err != nil {
		return err
	}

	// Repeat a fetch only when an invalidation supersedes it.
	for {
		var gen uint64
		a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			gen = a.getOrCreateSharedObjectMetadataStateLocked(soID).gen
		})

		// Fetch and decode metadata for this shared object.
		data, err := cli.GetSOMetadata(ctx, soID)
		if err != nil {
			return errors.Wrap(err, "get shared object metadata")
		}
		metadata := &api.SpaceMetadataResponse{}
		if err := metadata.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "unmarshal shared object metadata")
		}

		// Publish this fetch only if its cache generation still matches.
		var retry bool
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			state := a.getOrCreateSharedObjectMetadataStateLocked(soID)
			if state.gen != gen {
				retry = state.status == sharedObjectMetadataInvalid
				return
			}
			setSharedObjectMetadataLocked(state, metadata, broadcast)
		})
		if !retry {
			return nil
		}
	}
}

// UpdateSharedObjectMetadata updates cloud metadata and stores the returned snapshot.
func (a *ProviderAccount) UpdateSharedObjectMetadata(
	ctx context.Context,
	soID string,
	metadata *api.SpaceMetadataResponse,
) (*api.SpaceMetadataResponse, error) {
	// Send the update with the account's session client.
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}
	cli, err := a.GetSessionClient()
	if err != nil {
		return nil, err
	}
	data, err := cli.UpdateSOMetadata(ctx, soID, metadata)
	if err != nil {
		return nil, errors.Wrap(err, "update shared object metadata")
	}

	// Store the returned snapshot in the cache and the list.
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
		setSharedObjectMetadataLocked(
			a.getOrCreateSharedObjectMetadataStateLocked(soID),
			metadata,
			broadcast,
		)
	})
}

// setSharedObjectMetadataLocked stores a valid metadata snapshot in state.
// Caller must hold accountBcast.
func setSharedObjectMetadataLocked(
	state *sharedObjectMetadataState,
	metadata *api.SpaceMetadataResponse,
	broadcast func(),
) {
	// Supersede in-flight fetches, and announce only a changed snapshot.
	state.gen++
	next := cloneSharedObjectMetadata(metadata)
	if state.status == sharedObjectMetadataValid && sharedObjectMetadataEqual(state.metadata, next) {
		return
	}
	state.metadata = next
	state.status = sharedObjectMetadataValid
	broadcast()
}

// InvalidateSharedObjectMetadata marks one shared-object metadata cache entry stale.
func (a *ProviderAccount) InvalidateSharedObjectMetadata(
	soID string,
) {
	if soID == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Supersede in-flight fetches and mark a cached entry stale.
		state := a.state.sharedObjectMetadata[soID]
		if state == nil {
			return
		}
		state.gen++
		if state.status == sharedObjectMetadataInvalid {
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
			if state == nil {
				continue
			}
			state.gen++
			if state.status == sharedObjectMetadataInvalid ||
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

// reconcileSharedObjectMetadata marks cached metadata stale when list shows a
// newer revision, does not list the object, or the cached revision is unknown.
// In-flight fetches of entries already stale repeat, since they may predate
// the list.
func (a *ProviderAccount) reconcileSharedObjectMetadata(list *sobject.SharedObjectList) {
	// Index the listed revisions.
	listed := make(map[string]int64, len(list.GetSharedObjects()))
	for _, entry := range list.GetSharedObjects() {
		listed[entry.GetRef().GetProviderResourceRef().GetId()] = entry.GetUpdatedAt()
	}

	// Mark each cached entry stale unless it holds the listed revision.
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Deleted entries stay deleted; stale ones only repeat their fetch.
		changed := false
		for soID, state := range a.state.sharedObjectMetadata {
			if state == nil || state.status == sharedObjectMetadataDeleted {
				continue
			}
			if state.status == sharedObjectMetadataInvalid {
				state.gen++
				continue
			}

			// Keep a valid entry the list confirms current.
			held := state.metadata.GetUpdatedAt()
			rev, ok := listed[soID]
			if ok && held != 0 && rev <= held {
				continue
			}
			state.gen++
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
		// Supersede in-flight fetches and record the deletion.
		state := a.getOrCreateSharedObjectMetadataStateLocked(soID)
		state.gen++
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
