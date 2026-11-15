package resource_world

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// undecodedRefusal returns nil when a client whose request names known as its
// known_error_code decodes code, and otherwise the error code stands for,
// which the resource RPC returns as the call's error. A client built before
// code existed thus fails the call instead of reading a refusal as success.
func undecodedRefusal(known, code s4wave_world.WorldErrorCode) error {
	if s4wave_world.Decodes(known, code) {
		return nil
	}
	return s4wave_world.ErrorFromCode(code)
}

// replayWaitCode returns REPLAY_WAITS when err is a replay wait and a client
// whose request names known decodes that code, so the handler answers with
// the code and writes nothing. Otherwise it returns err, which the resource
// RPC returns as the call's error: a client built before REPLAY_WAITS existed
// fails the call instead of reading the refused write as success.
func replayWaitCode(known s4wave_world.WorldErrorCode, err error) (s4wave_world.WorldErrorCode, error) {
	code := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS
	if errors.Is(err, sobject.ErrReplayWaits) && s4wave_world.Decodes(known, code) {
		return code, nil
	}
	return 0, err
}
