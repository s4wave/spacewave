package space_world

import (
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// replayRefusals are the World engine's errors that refuse an operation on
// replay. Each depends only on the operation and the World it applies to, so
// every replica that applies the operation on the same World refuses it.
var replayRefusals = []error{
	world.ErrEngineUnguardedOp,
	world.ErrObjectExists,
	world.ErrObjectNotFound,
	world.ErrQuadNotFound,
	world.ErrEmptyObjectKey,
	world.ErrEmptyOp,
	world.ErrEmptyChange,
	world.ErrEmptyEngineID,
	world.ErrUnexpectedChangeType,
	world.ErrUnexpectedRev,
	world.ErrGraphPathResultLimit,
	world.ErrGraphPathStepLimit,
	world.ErrGraphPathPredicate,
	world.ErrGraphPathDirection,
	world.ErrGraphEdgeBucketLimit,
	world.ErrGraphEdgeBucketDirection,
	world.ErrNilQuad,
	world.ErrEmptyQuadSubject,
	world.ErrEmptyQuadPred,
	world.ErrEmptyQuadObject,
	world.ErrQuadSubjectNotIRI,
	world.ErrQuadObjectNotIRI,
	world.ErrNotIRI,
}

// replayRefuses reports whether err, the failure of an operation's apply on
// replay, refuses the operation. Any other failure stops replay at the
// operation until it can apply it.
//
// Invariant: replay refuses only on a failure of a type that depends on the
// operation and its World alone, and never on one its handler reported as a
// system error: a guard's or a plugin handler's *world.OperationRejection, a
// native handler's or a validation's *world.OpRefusedError, or one of
// replayRefusals. A refusal is final on every replica, so refusing on a
// failure that depends on this device, such as a missing or corrupt block, a
// read the device could not finish, a store it opened read only or a handler
// it lacks, would split replicas. A failure of a wait class, or of
// no known type, therefore waits, even when it wraps a refusal.
func replayRefuses(err error) bool {
	// Wait on a failure that depends on this device.
	var sysErr *world.OpSystemError
	var panicked *world.OpPanicError
	if err == nil || errors.As(err, &sysErr) || isReplayGap(err) || errors.As(err, &panicked) ||
		errors.Is(err, block.ErrBlockCorrupt) || errors.Is(err, tx.ErrDiscarded) ||
		errors.Is(err, world.ErrDurabilityUnknown) || world.IsSuperseded(err) ||
		errors.Is(err, world.ErrHistoryUnavailable) ||
		errors.Is(err, world.ErrWorldStorageUnavailable) ||
		errors.Is(err, world.ErrStateNotFound) {
		return false
	}

	// Refuse on a failure the operation and its World decide.
	var rejection *world.OperationRejection
	var refused *world.OpRefusedError
	if errors.As(err, &rejection) || errors.As(err, &refused) {
		return true
	}
	for _, refusal := range replayRefusals {
		if errors.Is(err, refusal) {
			return true
		}
	}
	return false
}
