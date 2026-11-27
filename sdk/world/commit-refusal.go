package s4wave_world

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
)

// commitRefusals pairs each typed commit refusal with its sentinel, in the
// order a server classifies a commit error. ErrStaleGeneration precedes
// ErrSuperseded because world.IsSuperseded matches both.
var commitRefusals = []struct {
	code     WorldErrorCode
	sentinel error
}{
	{WorldErrorCode_WORLD_ERROR_CODE_ROOT_NOT_HELD, block.ErrRootNotHeld},
	{WorldErrorCode_WORLD_ERROR_CODE_STALE_GENERATION, coord.ErrStaleGeneration},
	{WorldErrorCode_WORLD_ERROR_CODE_SUPERSEDED, world.ErrSuperseded},
}

// NewCommitResponse returns the Commit result for a transaction's commit
// error, for a client whose request specifies known as its known_error_code.
// A nil error and a typed refusal return a response with a nil error. Any other
// error returns as the RPC error.
//
// A typed refusal means nothing was committed. A commit whose outcome is
// unknown is never a refusal, whatever else its error wraps, so it matches
// before any refusal: it returns DURABILITY_UNKNOWN to a client that decodes
// it, and otherwise the bare world.ErrDurabilityUnknown as the RPC error. The
// bare text keeps an older client, which classifies the error by its text,
// from reading a superseded commit the error wraps as one it may run again.
func NewCommitResponse(known WorldErrorCode, err error) (*CommitResponse, error) {
	// Answer a published commit with the zero response.
	if err == nil {
		return &CommitResponse{}, nil
	}

	// Answer an unknown outcome before any refusal it wraps.
	if errors.Is(err, world.ErrDurabilityUnknown) {
		code := WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN
		if !Decodes(known, code) {
			return nil, world.ErrDurabilityUnknown
		}
		return &CommitResponse{ErrorCode: code, ErrorMessage: err.Error()}, nil
	}

	// Answer a typed refusal with its code, and any other error as the call's.
	for _, r := range commitRefusals {
		if errors.Is(err, r.sentinel) {
			return &CommitResponse{ErrorCode: r.code, ErrorMessage: err.Error()}, nil
		}
	}
	return nil, err
}

// commitResponseError restores the typed refusal or unknown outcome a Commit
// response carries, or nil when the commit published. CommitRemote is its only
// caller, so every client reads a refusal as an error before it sees the
// response.
func commitResponseError(resp *CommitResponse) error {
	// Restore a published commit and an unknown outcome.
	code := resp.GetErrorCode()
	switch code {
	case WorldErrorCode_WORLD_ERROR_CODE_UNSPECIFIED:
		return nil
	case WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN:
		return &durabilityUnknownError{message: resp.GetErrorMessage()}
	}

	// Restore a typed refusal, and fail closed on a code this build lacks.
	for _, c := range commitRefusals {
		if c.code == code {
			return &CommitRefusal{Code: code, Message: resp.GetErrorMessage(), sentinel: c.sentinel}
		}
	}
	return errors.Errorf("commit refused with unknown code %v: %s", code, resp.GetErrorMessage())
}

// CommitRefusal is a commit the remote World refused with a typed result. It
// reads as the server's error and unwraps to the result's sentinel, so
// world.IsSuperseded and errors.Is(err, block.ErrRootNotHeld) match it.
type CommitRefusal struct {
	// Code is the typed result.
	Code WorldErrorCode
	// Message is the server's full error text.
	Message string
	// sentinel is the error Code stands for.
	sentinel error
}

// Error returns the server's error text.
func (e *CommitRefusal) Error() string {
	if e.Message == "" {
		return e.sentinel.Error()
	}
	return e.Message
}

// Unwrap returns the sentinel the refusal's code stands for.
func (e *CommitRefusal) Unwrap() error {
	return e.sentinel
}

// durabilityUnknownError is a remote commit whose outcome is unknown. It reads
// as the server's error and unwraps to world.ErrDurabilityUnknown, so a caller
// that matches the sentinel treats it as final.
type durabilityUnknownError struct {
	// message is the server's full error text.
	message string
}

// Error returns the server's error text.
func (e *durabilityUnknownError) Error() string {
	if e.message == "" {
		return world.ErrDurabilityUnknown.Error()
	}
	return e.message
}

// Unwrap returns world.ErrDurabilityUnknown.
func (e *durabilityUnknownError) Unwrap() error {
	return world.ErrDurabilityUnknown
}
