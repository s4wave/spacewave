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
// error. A nil error and a typed refusal return a response with a nil error;
// any other error returns as the RPC error.
//
// A typed refusal means nothing was committed. A commit whose outcome is
// unknown is never a refusal, whatever else its error wraps, so
// world.ErrDurabilityUnknown returns as the RPC error before any refusal
// matches.
func NewCommitResponse(err error) (*CommitResponse, error) {
	if err == nil {
		return &CommitResponse{}, nil
	}
	if errors.Is(err, world.ErrDurabilityUnknown) {
		return nil, err
	}
	for _, r := range commitRefusals {
		if errors.Is(err, r.sentinel) {
			return &CommitResponse{ErrorCode: r.code, ErrorMessage: err.Error()}, nil
		}
	}
	return nil, err
}

// commitResponseError restores the typed refusal a Commit response carries,
// or nil when the commit published. CommitRemote is its only caller, so every
// client reads a refusal as an error before it sees the response.
func commitResponseError(resp *CommitResponse) error {
	code := resp.GetErrorCode()
	if code == WorldErrorCode_WORLD_ERROR_CODE_UNSPECIFIED {
		return nil
	}
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
