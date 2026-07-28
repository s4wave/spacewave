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

// ObjectBodiesBatchRevisionError reports that paginated reads crossed World
// sequence numbers too many times to form one consistent result.
type ObjectBodiesBatchRevisionError struct {
	// Expected is the sequence number recorded from the first page.
	Expected uint64
	// Got is the sequence number observed on the mismatching page.
	Got uint64
	// Retries is the number of pagination restarts attempted.
	Retries int
}

// Error returns the inconsistent World sequence details.
func (e *ObjectBodiesBatchRevisionError) Error() string {
	return errors.Errorf(
		"object bodies batch World sequence changed from %d to %d after %d retries",
		e.Expected,
		e.Got,
		e.Retries,
	).Error()
}
