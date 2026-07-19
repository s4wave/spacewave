package s4wave_world

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ErrorFromCode restores the sentinel error represented by a World RPC code.
func ErrorFromCode(code WorldErrorCode) error {
	switch code {
	case WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP:
		return errors.Wrap(world.ErrUnhandledOp, "remote world operation unhandled")
	default:
		return nil
	}
}
