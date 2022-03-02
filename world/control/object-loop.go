package world_control

import (
	"context"
	"sync"

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
	// objectKey is the object to monitor
	objectKey string
	// handler is the object loop handler
	handler ObjectLoopHandler

	// mtx guards below fields
	mtx sync.Mutex
	// wake can be called to force re-scan
	// may be nil
	wake func()
}

// ObjectLoopHandler is the callback function for the ObjectLoop.
// le may be nil
type ObjectLoopHandler = func(
	ctx context.Context,
	le *logrus.Entry,
	world world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error)

// NewObjectLoop constructs a new Control Loop which looks up an Engine on
// the Bus, looks up an Object, and calls the Callback when the state changes.
//
// le may be nil
func NewObjectLoop(
	le *logrus.Entry,
	objectKey string,
	handler ObjectLoopHandler,
) *ObjectLoop {
	return &ObjectLoop{
		le:        le,
		objectKey: objectKey,
		handler:   handler,
	}
}

// NewBusObjectLoop constructs a new BusEngine which attaches to an engine
// running on a controller bus.
func NewBusObjectLoop(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engineID string, write bool,
	objectKey string, handler ObjectLoopHandler,
) (*ObjectLoop, *world.BusEngine, world.WorldState) {
	busEngine := world.NewBusEngine(ctx, b, engineID)
	ws := world.NewEngineWorldState(ctx, busEngine, true)
	return NewObjectLoop(le, objectKey, handler), busEngine, ws
}

// ExecuteBusObjectLoop executes an existing ObjectLoop with a Bus engine.
func ExecuteBusObjectLoop(
	ctx context.Context,
	b bus.Bus,
	engineID string, write bool,
	objLoop *ObjectLoop,
) error {
	busEngine := world.NewBusEngine(ctx, b, engineID)
	ws := world.NewEngineWorldState(ctx, busEngine, true)
	return objLoop.Execute(ctx, ws)
}

// Wake forces the control loop to re-process the latest object state.
func (c *ObjectLoop) Wake() {
	c.mtx.Lock()
	if wake := c.wake; wake != nil {
		wake()
		c.wake = nil
	}
	c.mtx.Unlock()
}

// Execute runs the ControlLoop execution loop.
func (c *ObjectLoop) Execute(ctx context.Context, ws world.WorldState) error {
	if c == nil || c.handler == nil {
		return nil
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	for {
		var rootRef *bucket.ObjectRef
		var rev uint64

		select {
		case <-subCtx.Done():
			return context.Canceled
		default:
		}

		seqno, err := ws.GetSeqno()
		if err != nil {
			return err
		}

		objState, objFound, err := ws.GetObject(c.objectKey)
		if err != nil {
			return err
		}
		if objFound {
			rootRef, rev, err = objState.GetRootRef()
			if err != nil {
				return err
			}
			if c.le != nil {
				c.le.
					WithField("object-id", c.objectKey).
					Debugf("object found at revision %d", rev)
			}
		} else {
			objState = nil
		}

		waitForChanges, err := c.handler(
			ctx, c.le,
			ws, objState,
			rootRef, rev,
		)
		if err != nil && c.le != nil {
			c.le.
				WithError(err).
				WithField("object-key", c.objectKey).
				WithField("world-seqno", seqno).
				WithField("wait-for-changes", waitForChanges).
				Warn("handler returned error")
		}
		if !waitForChanges {
			return err
		}

		wakeCtx, wakeCtxCancel := context.WithCancel(subCtx)
		c.mtx.Lock()
		c.wake = wakeCtxCancel
		c.mtx.Unlock()

		if objState != nil {
			_, err = objState.WaitRev(wakeCtx, rev+1, !objFound)
			if err == world.ErrObjectNotFound && objFound {
				// ignore ErrObjectNotFound if we previously found the object
				// allow the handler to be notified of the deletion
				err = nil
			}
		} else {
			_, err = ws.WaitSeqno(wakeCtx, seqno+1)
		}
		wakeCtxCancel()
		if err != nil && err != context.Canceled {
			return err
		}
	}
}
