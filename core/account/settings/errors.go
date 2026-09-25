package account_settings

import (
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrStorageBackendNotFound is returned when a storage backend id is unknown.
	ErrStorageBackendNotFound = errors.New("storage backend not found")
	// ErrStorageBackendInUse matches a StorageBackendInUseError.
	ErrStorageBackendInUse = errors.New("storage backend holds Spaces")
)

// StorageBackendInUseError is returned when removing a backend that holds
// Spaces. It matches ErrStorageBackendInUse.
type StorageBackendInUseError struct {
	// Spaces names each placed Space, or its block store id when unnamed.
	Spaces []string
}

// Error names the Spaces the backend holds.
func (e *StorageBackendInUseError) Error() string {
	return ErrStorageBackendInUse.Error() + ": " + strings.Join(e.Spaces, ", ")
}

// Is reports whether target is ErrStorageBackendInUse.
func (e *StorageBackendInUseError) Is(target error) bool {
	return target == ErrStorageBackendInUse
}
