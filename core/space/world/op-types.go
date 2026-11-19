package space_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// OpTypes resolves the World operation types a Space engine applies.
type OpTypes struct {
	// Lookup resolves built-in operations before the bus lookup, or is nil.
	// It is the static schema every release of this binary shares.
	Lookup world.LookupOp
}

// ErrBusPayloadOperation is returned when a handler on the bus resolves an
// operation type to a world.PayloadOperation. Only OpTypes.Lookup may name
// payload roots, so the payload hold knows every type's PayloadRefsVersion
// without asking the bus (payloadVersion).
var ErrBusPayloadOperation = errors.New("a bus operation handler names payload roots")

// payloadVersion returns the PayloadRefsVersion of the operation Lookup
// resolves for opTypeID, or zero when Lookup resolves none or one that names
// no payload roots of its own. Every other type names only the object roots of
// its transaction, since LookupOn rejects a payload operation from the bus.
func (t OpTypes) payloadVersion(ctx context.Context, opTypeID string) (uint32, error) {
	// Resolve the type in the static table alone.
	if t.Lookup == nil {
		return 0, nil
	}
	op, err := t.Lookup(ctx, opTypeID)
	if err != nil {
		return 0, err
	}

	// Read the version of a payload operation.
	if pop, ok := op.(world.PayloadOperation); ok {
		return pop.PayloadRefsVersion(), nil
	}
	return 0, nil
}

// objectRootsOnly returns lookup rejecting a world.PayloadOperation with
// ErrBusPayloadOperation.
func objectRootsOnly(lookup world.LookupOp) world.LookupOp {
	return func(ctx context.Context, opTypeID string) (world.Operation, error) {
		op, err := lookup(ctx, opTypeID)
		if _, ok := op.(world.PayloadOperation); ok && err == nil {
			return nil, errors.Wrap(ErrBusPayloadOperation, opTypeID)
		}
		return op, err
	}
}

// LookupOn returns the lookup a Space's Adapter applies operations with: Lookup
// first, then the handlers registered on b for the World engine engineID,
// such as a running plugin's, which may not name payload roots
// (ErrBusPayloadOperation). A nil b resolves with Lookup alone. A type neither
// resolves resolves to nothing.
func (t OpTypes) LookupOn(b bus.Bus, le *logrus.Entry, engineID string) world.LookupOp {
	var lookups world.LookupOpSlice
	if t.Lookup != nil {
		lookups = append(lookups, t.Lookup)
	}
	if b != nil {
		lookups = append(lookups, objectRootsOnly(world.BuildLookupWorldOpFunc(b, le, engineID)))
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
		lookups = append(lookups, objectRootsOnly(held))
	}
	return lookups.LookupOp, release
}

// unresolvedOpError reports an operation whose type neither the static
// schema nor a handler on the bus resolves on this device yet.
//
// Invariant: the author and every replayer decide a position alike from the
// operation and the World alone. Whether a handler exists is a fact about the
// device and its release, so a missing handler is never a refusal: replay
// waits at the operation until a handler registers.
type unresolvedOpError struct {
	// opTypeID is the operation type.
	opTypeID string
	// pluginID is the plugin a pinned type names, or empty.
	pluginID string
	// pluginIDs are the plugins to run while waiting: the pinned one, or
	// every plugin the World before the operation lists.
	pluginIDs []string
}

// Error returns the error message.
func (e *unresolvedOpError) Error() string {
	return "no handler resolves operation type " + e.opTypeID
}

// ErrUnreadableOp is returned at an operation whose body this build cannot
// read: a World operation a newer build wrote, with a body this build's
// WorldOp does not declare, or with the removed body an older build wrote.
// Replay waits at it for a build that reads it, and ImportRoots cannot name
// its payloads.
var ErrUnreadableOp = errors.New("this build cannot read the operation's body")

// neverWakes is the Wake of a replay wait only a restart ends. Nothing closes
// it.
var neverWakes = make(chan struct{})

// unreadable reports whether wop decoded without a body this build declares
// but with fields it does not: a body this build cannot read. An empty
// WorldOp decodes alike on every build, so replay refuses it instead.
func unreadable(wop *WorldOp) bool {
	return wop.GetBody() == nil && len(wop.unknownFields) != 0
}

// isReplayGap reports whether err stops replay at its operation instead of
// deciding the operation: a block that is not available, a canceled read, a
// put through a replay stage that was released or lost, an operation whose
// type no handler resolves yet, or a plugin this device could not reach or
// has no build of. None is a property of the operation, so refusing it would
// split replicas.
func isReplayGap(err error) bool {
	var unresolved *unresolvedOpError
	var unavailable *world.PluginUnavailableError
	var missing *world.PluginExecutableMissingError
	return block.IsNotAvailable(err) ||
		errors.Is(err, block.ErrStageReleased) ||
		errors.As(err, &unresolved) ||
		errors.As(err, &unavailable) ||
		errors.As(err, &missing) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// waitingLookup wraps lookupOp so that an operation type it does not
// resolve fails with an unresolvedOpError instead of resolving to nothing. ws
// is the World the operation runs on, whose Space settings name the plugins
// to run while replay waits.
func waitingLookup(lookupOp world.LookupOp, ws world.WorldState) world.LookupOp {
	return func(ctx context.Context, opTypeID string) (world.Operation, error) {
		// Resolve the operation.
		op, err := lookupOp(ctx, opTypeID)
		if op != nil || err != nil {
			return op, err
		}

		// Name the plugins that could handle it: the pinned one, or the
		// listed ones. A settings read that fails for want of a block stops
		// replay there; any other failure only names no plugin.
		wait := &unresolvedOpError{opTypeID: opTypeID}
		if pluginID, _, _, ok := s4wave_worldop_registry.ParsePinnedOperationID(opTypeID); ok {
			wait.pluginID, wait.pluginIDs = pluginID, []string{pluginID}
			return nil, wait
		}
		settings, err := LookupSpaceSettingsBody(ctx, ws)
		if isReplayGap(err) {
			return nil, err
		}
		wait.pluginIDs = slices.Sorted(slices.Values(settings.GetPluginIds()))
		return nil, wait
	}
}
