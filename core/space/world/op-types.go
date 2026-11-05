package space_world

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// OpTypes resolves the World operation types a Space engine applies.
type OpTypes struct {
	// Lookup resolves built-in operations before the bus lookup, or is nil.
	Lookup world.LookupOp
	// Declared returns the plugins the World ws declares handle opTypeID, an
	// operation type neither Lookup nor the bus resolves. Replay stops at an
	// operation of a declared type until a handler for it resolves on the bus,
	// and rejects one of an undeclared type. It must read only ws, so every
	// replica decides alike. Nil declares none.
	Declared func(ctx context.Context, ws world.WorldState, opTypeID string) ([]string, error)
}

// pendingOpError reports an operation whose type the World declares a plugin
// handles, met before a handler for the type resolved.
type pendingOpError struct {
	// opTypeID is the operation type.
	opTypeID string
	// pluginIDs are the plugins the World declares handle it.
	pluginIDs []string
}

// Error returns the error message.
func (e *pendingOpError) Error() string {
	return "operation type " + e.opTypeID + " waits for a handler from plugin " + strings.Join(e.pluginIDs, ", ")
}

// isReplayGap reports whether err stops replay at its operation instead of
// deciding the operation: a block that is not available, a canceled read, or
// an operation whose declared handler has not resolved. None is a property of
// the operation, so refusing it would split replicas.
func isReplayGap(err error) bool {
	var pending *pendingOpError
	return block.IsNotAvailable(err) ||
		errors.As(err, &pending) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// declaredLookup wraps lookupOp so that an operation type it does not
// resolve, and that ws declares a plugin handles, fails with a pendingOpError
// instead of resolving to nothing.
func declaredLookup(lookupOp world.LookupOp, declared func(context.Context, world.WorldState, string) ([]string, error), ws world.WorldState) world.LookupOp {
	if declared == nil {
		return lookupOp
	}
	return func(ctx context.Context, opTypeID string) (world.Operation, error) {
		// Resolve the operation, then check an unresolved type's declaration.
		op, err := lookupOp(ctx, opTypeID)
		if op != nil || err != nil {
			return op, err
		}
		pluginIDs, err := declared(ctx, ws, opTypeID)
		if err != nil || len(pluginIDs) == 0 {
			return nil, err
		}
		return nil, &pendingOpError{opTypeID: opTypeID, pluginIDs: pluginIDs}
	}
}
