//go:build !js && !wasip1

package spacewave_cli

import (
	"path/filepath"

	"github.com/pkg/errors"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/daemon"
)

// StatePathLeaseHeldError reports the process that owns a writable state path.
type StatePathLeaseHeldError struct {
	// StatePath is the canonical writable root.
	StatePath string
	// HolderPID identifies the lease holder when available.
	HolderPID int
	// StorePath identifies the contended storage lock.
	StorePath string
}

// Error returns the terminal writable-state conflict.
func (e *StatePathLeaseHeldError) Error() string {
	if e.HolderPID > 0 && e.StorePath != "" {
		return errors.Errorf("writable state path %s is held by PID %d through store %s", e.StatePath, e.HolderPID, e.StorePath).Error()
	}
	if e.HolderPID > 0 {
		return errors.Errorf("writable state path %s is held by PID %d", e.StatePath, e.HolderPID).Error()
	}
	if e.StorePath != "" {
		return errors.Errorf("writable state path %s is held by a writer-capable process through store %s", e.StatePath, e.StorePath).Error()
	}
	return errors.Errorf("writable state path %s is held by another process", e.StatePath).Error()
}

// Is distinguishes a competing daemon startup from a separate writable store.
func (e *StatePathLeaseHeldError) Is(target error) bool {
	return target == daemon.ErrStarting && filepath.Base(e.StorePath) == statePathLeaseStorageID+storage_native.BoltDBExt
}
