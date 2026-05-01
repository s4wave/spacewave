package provider_spacewave

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

var errSharedObjectInitialStateRejected = errors.New(
	"shared object initial state rejected",
)

var errSharedObjectCurrentKeyEpochMissing = errors.New(
	"current key epoch missing for self-enroll recovery",
)

// isTerminalSharedObjectMountError returns true when err should be surfaced to
// mount waiters without re-entering keyed retry.
func isTerminalSharedObjectMountError(err error) bool {
	if errors.Is(err, errSharedObjectInitialStateRejected) {
		return true
	}
	if errors.Is(err, errSharedObjectCurrentKeyEpochMissing) {
		return true
	}
	if errors.Is(err, sobject.ErrNotParticipant) {
		return true
	}
	return isCloudAccessGatedError(err)
}
