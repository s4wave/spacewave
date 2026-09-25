package account_settings

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/space"
)

// FindStorageBackend returns the storage backend with the id, or nil.
func (s *AccountSettings) FindStorageBackend(id string) *StorageBackend {
	for _, backend := range s.GetStorageBackends() {
		if backend.GetId() == id {
			return backend
		}
	}
	return nil
}

// FindStorageBackendByName returns the storage backend with the display
// name, or nil.
func (s *AccountSettings) FindStorageBackendByName(name string) *StorageBackend {
	for _, backend := range s.GetStorageBackends() {
		if backend.GetDisplayName() == name {
			return backend
		}
	}
	return nil
}

// FindBlockStorePlacement returns the placement of the block store, or nil
// when the block store is on the account's own storage.
func (s *AccountSettings) FindBlockStorePlacement(blockStoreID string) *BlockStorePlacement {
	for _, placement := range s.GetBlockStorePlacements() {
		if placement.GetBlockStoreId() == blockStoreID {
			return placement
		}
	}
	return nil
}

// PlacedBlockStoreIDs returns the ids of the block stores placed on the
// storage backend.
func (s *AccountSettings) PlacedBlockStoreIDs(backendID string) []string {
	var ids []string
	for _, placement := range s.GetBlockStorePlacements() {
		if placement.GetStorageBackendId() == backendID {
			ids = append(ids, placement.GetBlockStoreId())
		}
	}
	return ids
}

// CheckStorageBackendUnused returns a StorageBackendInUseError naming the
// Spaces placed on the backend, or nil when it holds none.
func (s *AccountSettings) CheckStorageBackendUnused(backendID string) error {
	placed := s.PlacedBlockStoreIDs(backendID)
	if len(placed) == 0 {
		return nil
	}
	spaces := make([]string, len(placed))
	for i, blockStoreID := range placed {
		spaces[i] = blockStoreID
		if _, name := s.FindSpaceByBlockStore(blockStoreID); name != "" {
			spaces[i] = name
		}
	}
	return &StorageBackendInUseError{Spaces: spaces}
}

// upsertStorageBackend adds a storage backend or replaces the one with its id.
func (s *AccountSettings) upsertStorageBackend(backend *StorageBackend) error {
	if err := backend.Validate(); err != nil {
		return err
	}
	if other := s.FindStorageBackendByName(backend.GetDisplayName()); other != nil && other.GetId() != backend.GetId() {
		return errors.Errorf("a storage backend named %q already exists", backend.GetDisplayName())
	}
	s.StorageBackends = slices.DeleteFunc(s.StorageBackends, func(current *StorageBackend) bool {
		return current.GetId() == backend.GetId()
	})
	s.StorageBackends = append(s.StorageBackends, backend.CloneVT())
	return nil
}

// removeStorageBackend removes a storage backend that holds no block store.
// Removing the default backend returns new Spaces to the account's storage.
// Pending releases on the backend are dropped with it.
func (s *AccountSettings) removeStorageBackend(id string) error {
	if id == "" {
		return errors.New("storage_backend_id is required")
	}
	if err := s.CheckStorageBackendUnused(id); err != nil {
		return err
	}
	s.StorageBackends = slices.DeleteFunc(s.StorageBackends, func(current *StorageBackend) bool {
		return current.GetId() == id
	})
	s.StorageReleases = slices.DeleteFunc(s.StorageReleases, func(release *BlockStorePlacement) bool {
		return release.GetStorageBackendId() == id
	})
	if s.GetDefaultStorageBackendId() == id {
		s.DefaultStorageBackendId = ""
	}
	return nil
}

// setDefaultStorageBackend selects the default storage backend, or the
// account's own storage when id is empty.
func (s *AccountSettings) setDefaultStorageBackend(id string) error {
	if id != "" && s.FindStorageBackend(id) == nil {
		return ErrStorageBackendNotFound
	}
	s.DefaultStorageBackendId = id
	return nil
}

// setBlockStorePlacement places a block store on a storage backend, or
// returns it to the account's own storage when the backend id is empty.
//
// Leaving a backend releases the store's objects there. Returning to a
// backend cancels its pending release, since the store writes there again.
func (s *AccountSettings) setBlockStorePlacement(placement *BlockStorePlacement) error {
	blockStoreID := placement.GetBlockStoreId()
	if blockStoreID == "" {
		return errors.New("block_store_id is required")
	}
	backendID := placement.GetStorageBackendId()
	if backendID != "" && s.FindStorageBackend(backendID) == nil {
		return ErrStorageBackendNotFound
	}
	s.unplaceBlockStore(blockStoreID)
	if backendID != "" {
		s.BlockStorePlacements = append(s.BlockStorePlacements, placement.CloneVT())
		s.dropStorageRelease(placement)
	}
	return nil
}

// unplaceBlockStore removes the block store's placement and releases its
// objects on the backend it leaves.
func (s *AccountSettings) unplaceBlockStore(blockStoreID string) {
	placement := s.FindBlockStorePlacement(blockStoreID)
	if placement == nil {
		return
	}
	s.BlockStorePlacements = slices.DeleteFunc(s.BlockStorePlacements, func(current *BlockStorePlacement) bool {
		return current.GetBlockStoreId() == blockStoreID
	})
	if !slices.ContainsFunc(s.StorageReleases, placement.EqualVT) {
		s.StorageReleases = append(s.StorageReleases, placement)
	}
}

// completeStorageRelease records that a released block store's objects are
// gone from the backend's bucket.
func (s *AccountSettings) completeStorageRelease(release *BlockStorePlacement) error {
	if release.GetBlockStoreId() == "" || release.GetStorageBackendId() == "" {
		return errors.New("block_store_id and storage_backend_id are required")
	}
	s.dropStorageRelease(release)
	return nil
}

// dropStorageRelease removes a pending release, if any.
func (s *AccountSettings) dropStorageRelease(release *BlockStorePlacement) {
	s.StorageReleases = slices.DeleteFunc(s.StorageReleases, release.EqualVT)
}

// Validate checks that the storage backend is complete.
func (b *StorageBackend) Validate() error {
	if b.GetId() == "" {
		return errors.New("storage backend id is required")
	}
	if b.GetDisplayName() == "" {
		return errors.New("storage backend display_name is required")
	}
	if err := b.GetS3().Validate(); err != nil {
		return errors.Wrap(err, "s3")
	}
	if err := b.GetCredential().GetRef().Validate(); err != nil {
		return errors.Wrap(err, "credential")
	}
	return nil
}

// Validate checks that the S3 location names an endpoint and bucket.
func (l *S3Location) Validate() error {
	if l.GetEndpoint() == "" {
		return errors.New("endpoint is required")
	}
	if l.GetBucket() == "" {
		return errors.New("bucket is required")
	}
	return nil
}

// BlockStorePrefix returns the key prefix of the block store's objects.
func (l *S3Location) BlockStorePrefix(blockStoreID string) string {
	return l.GetObjectPrefix() + blockStoreID + "/"
}

// FindSpaceByBlockStore returns the id and name of the cataloged Space whose
// blocks live in the block store. Both are empty when no live Space uses it.
func (s *AccountSettings) FindSpaceByBlockStore(blockStoreID string) (spaceID, name string) {
	for _, catalogEntry := range s.GetCatalog() {
		entry := catalogEntry.GetEntry()
		if catalogEntry.GetDeleted() || entry.GetRef().GetBlockStoreId() != blockStoreID {
			continue
		}
		meta := entry.GetMeta()
		if meta.GetBodyType() != space.SpaceBodyType {
			continue
		}
		spaceMeta := &space.SpaceSoMeta{}
		if err := spaceMeta.UnmarshalVT(meta.GetBodyMeta()); err != nil {
			continue
		}
		return entry.GetRef().GetProviderResourceRef().GetId(), spaceMeta.GetName()
	}
	return "", ""
}
