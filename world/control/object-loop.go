package world_control

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// ObjectLoop is a utility for building Controllers which bind to world graph
// Objects, running reconciliation loops until the Object reaches desired state.
type ObjectLoop struct {
	// le is the logger
	le *logrus.Entry
	// engine is the world engine
	engine world.Engine
	// objectID is the object to monitor
	objectID string
	// handler is the object loop handler
	handler ObjectLoopHandler
	// write indicate if writes are allowed
	write bool
}

// ObjectLoopHandler is the callback function for the ObjectLoop.
type ObjectLoopHandler = func(
	ctx context.Context,
	le *logrus.Entry,
	engine world.Engine,
	world world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error)

// NewObjectLoop constructs a new Control Loop which looks up an Engine on
// the Bus, looks up an Object, and calls the Callback when the state changes.
func NewObjectLoop(le *logrus.Entry, eng world.Engine, write bool, objectID string, handler ObjectLoopHandler) *ObjectLoop {
	return &ObjectLoop{
		le:       le,
		engine:   eng,
		objectID: objectID,
		handler:  handler,
		write:    write,
	}
}

// NewBusObjectLoop constructs a new BusEngine which attaches to an engine
// running on a controller bus.
func NewBusObjectLoop(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engineID string, write bool,
	objectID string, handler ObjectLoopHandler,
) (*ObjectLoop, *world.BusEngine) {
	busEngine := world.NewBusEngine(ctx, b, engineID)
	return NewObjectLoop(le, busEngine, write, objectID, handler), busEngine
}

// Execute runs the ControlLoop execution loop.
func (c *ObjectLoop) Execute(ctx context.Context) error {
	if c == nil || c.handler == nil {
		return nil
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	worldState := world.NewEngineWorldState(subCtx, c.engine, c.write)
	for {
		var rootRef *bucket.ObjectRef
		var rev uint64
		objState, objFound, err := worldState.GetObject(c.objectID)
		if err != nil {
			return err
		}
		if objFound {
			rootRef, rev, err = objState.GetRootRef()
			if err != nil {
				return err
			}
			c.le.
				WithField("object-id", c.objectID).
				Debugf("object found at revision %d", rev)
		} else {
			objState = nil
		}

		waitForChanges, err := c.handler(
			ctx, c.le,
			c.engine,
			worldState, objState,
			rootRef, rev,
		)
		if err != nil {
			c.le.
				WithError(err).
				WithField("wait-for-changes", waitForChanges).
				Warn("handler returned error")
		}
		if !waitForChanges {
			return err
		}

		_, err = objState.WaitRev(ctx, rev+1, !objFound)
		if err == world.ErrObjectNotFound && objFound {
			// ignore ErrObjectNotFound if we previously found the object
			// allow the handler to be notified of the deletion
			err = nil
		}
		if err != nil {
			return err
		}
	}
}
