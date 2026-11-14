package space_world

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
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

// LookupOn returns the lookup a Space's Adapter applies operations with: Lookup
// first, then the handlers registered on b for the World engine engineID,
// such as a running plugin's. A nil b resolves with Lookup alone. A type
// neither resolves resolves to nothing.
func (t OpTypes) LookupOn(b bus.Bus, le *logrus.Entry, engineID string) world.LookupOp {
	var lookups world.LookupOpSlice
	if t.Lookup != nil {
		lookups = append(lookups, t.Lookup)
	}
	if b != nil {
		lookups = append(lookups, world.BuildLookupWorldOpFunc(b, le, engineID))
	}
	return lookups.LookupOp
}

// HeldLookupOn returns the lookup LookupOn returns, with the bus lookup made
// once per type and held until release (world.BuildHeldLookupWorldOpFunc),
// for a walk over many operations that must see one set of handlers.
func (t OpTypes) HeldLookupOn(b bus.Bus, le *logrus.Entry, engineID string) (world.LookupOp, func()) {
	// Try the static lookup first, then the held bus lookup.
	var lookups world.LookupOpSlice
	if t.Lookup != nil {
		lookups = append(lookups, t.Lookup)
	}
	release := func() {}
	if b != nil {
		var held world.LookupOp
		held, release = world.BuildHeldLookupWorldOpFunc(b, le, engineID)
		lookups = append(lookups, held)
	}
	return lookups.LookupOp, release
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
// deciding the operation: a block that is not available, a canceled read, a
// put through a replay stage that was released or lost, or an operation whose
// declared handler has not resolved. None is a property of the operation, so
// refusing it would split replicas.
func isReplayGap(err error) bool {
	var pending *pendingOpError
	return block.IsNotAvailable(err) ||
		errors.Is(err, block.ErrStageReleased) ||
		errors.As(err, &pending) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// awaitOpHandler asks the holders of the Space's plugin runtime to run the
// plugins pending names, with a LoadOpPlugins directive held while it waits,
// and returns once a handler for pending's operation type resolves on the
// bus. Without a bus lookup no handler can resolve, so it waits for ctx.
func (a *Adapter) awaitOpHandler(ctx context.Context, pending *pendingOpError) error {
	// Wait out the run when nothing can resolve the type.
	if a.opts.DisableLookup || a.opts.Bus == nil {
		<-ctx.Done()
		return context.Cause(ctx)
	}

	// Ask for the declaring plugins while waiting.
	a.le.WithField("op-type", pending.opTypeID).WithField("plugin-ids", pending.pluginIDs).Info("replay waits for an operation plugin")
	_, loadRef, err := a.opts.Bus.AddDirective(ctx, NewLoadOpPlugins(a.opts.EngineID, pending.pluginIDs), nil)
	if err != nil {
		return err
	}
	defer loadRef.Release()

	// Wait for a lookup value that resolves the type. Other values, such as
	// the built-in operations, resolve nothing for it.
	_, _, ref, err := bus.ExecWaitValue[world.LookupWorldOpValue](
		ctx,
		a.opts.Bus,
		world.NewLookupWorldOp(pending.opTypeID, a.opts.EngineID),
		nil,
		nil,
		func(lookup world.LookupWorldOpValue) (bool, error) {
			op, err := lookup(ctx, pending.opTypeID)
			return op != nil && err == nil, nil
		},
	)
	if err != nil {
		return err
	}
	ref.Release()
	return nil
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
