package resource_world

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// undecodedRefusal returns nil when a client whose request declares known as
// its known_error_code decodes code, and otherwise returns the error
// represented by code, which the resource RPC returns as the call's error. A
// client built before code existed thus fails the call instead of reading a
// refusal as success.
func undecodedRefusal(known, code s4wave_world.WorldErrorCode) error {
	if s4wave_world.Decodes(known, code) {
		return nil
	}
	return s4wave_world.ErrorFromCode(code)
}

// writeErrorCode returns the code a write's error stands for when a client
// whose request declares known decodes the code, so the handler answers with
// the code: DURABILITY_UNKNOWN when the World may hold the write, and
// REPLAY_WAITS when the World wrote nothing because its replay is not caught
// up. Otherwise writeErrorCode returns err, which the resource RPC returns as
// the call's error, so a client built before the code existed fails the call
// instead of reading the refusal as success.
//
// An unknown outcome matches first, whatever else the outcome's error wraps,
// and a client that does not decode the outcome's code gets the bare
// world.ErrDurabilityUnknown, so no wrapped refusal's text suggests the client
// may send the write again.
func writeErrorCode(known s4wave_world.WorldErrorCode, err error) (s4wave_world.WorldErrorCode, error) {
	// Match an unknown outcome before any refusal it wraps.
	if errors.Is(err, world.ErrDurabilityUnknown) {
		code := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN
		if !s4wave_world.Decodes(known, code) {
			return 0, world.ErrDurabilityUnknown
		}
		return code, nil
	}

	// Match a replay wait the client decodes.
	code := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS
	if errors.Is(err, sobject.ErrReplayWaits) && s4wave_world.Decodes(known, code) {
		return code, nil
	}
	return 0, err
}
