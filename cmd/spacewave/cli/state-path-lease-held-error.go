//go:build !js && !wasip1

package spacewave_cli

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/daemon"
)

// StatePathLeaseHeldError reports a runtime holding a writable state path lease.
type StatePathLeaseHeldError struct {
	// StatePath is the canonical writable root.
	StatePath string
	// HolderPID identifies the holder for same-process conflicts.
	HolderPID int
	// StorePath identifies the runtime coordination store.
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
		return errors.Errorf("writable state path %s is held by another runtime through store %s", e.StatePath, e.StorePath).Error()
	}
	return errors.Errorf("writable state path %s is held by another process", e.StatePath).Error()
}

// Is identifies a competing daemon startup for socket readiness watchers.
func (e *StatePathLeaseHeldError) Is(target error) bool {
	return target == daemon.ErrStarting
}
