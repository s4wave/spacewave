package world_block

import (
	"errors"
	"fmt"

	"github.com/s4wave/spacewave/db/world"
)

// ErrEngineClosed is returned when an operation is attempted on a closed
// engine. Callers that shut the engine down alongside a context cancellation
// can observe either this or context.Canceled, depending on which unwinds
// first, so both are valid outcomes of the same shutdown.
var ErrEngineClosed = errors.New("world block engine is closed")

// errEngineRetired is what a retired engine and the transactions it ended
// report: the engine is closed, and the World moved to its successor, so a
// caller runs its change again there.
var errEngineRetired = fmt.Errorf("%w: %w", ErrEngineClosed, world.ErrEngineRetired)

// ErrAuthorityHead is returned by SetRootRef on an engine with an Authority,
// whose head only the Authority moves.
var ErrAuthorityHead = errors.New("world block engine head belongs to its authority")
