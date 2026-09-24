package account_settings

import "github.com/pkg/errors"

var (
	// ErrStorageBackendNotFound is returned when a storage backend id is unknown.
	ErrStorageBackendNotFound = errors.New("storage backend not found")
	// ErrStorageBackendInUse is returned when removing a backend that holds
	// block stores.
	ErrStorageBackendInUse = errors.New("storage backend holds block stores")
)
