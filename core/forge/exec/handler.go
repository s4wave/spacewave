package space_exec

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// Handler executes a single forge task within the space context.
// Receives inputs, performs work via the handle, sets outputs, returns.
// Returning nil signals successful completion. Returning an error signals failure.
type Handler interface {
	// Execute runs the handler to completion.
	Execute(ctx context.Context) error
}

// HandlerFactory constructs a Handler for a given exec config ID.
// No bus.Bus parameter: handlers access only the world state and exec handle.
type HandlerFactory func(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error)
