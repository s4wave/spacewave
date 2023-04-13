package world_control

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// WatchLoop is a utility for building Controllers which bind to world state,
// running reconciliation loops until the world reaches a desired state.
type WatchLoop struct {
	// le is the logger
	le *logrus.Entry
	// objectKey is the object to monitor (if any)
	// if unset monitors entire world state
	objectKey string
	// handler is the watch loop handler
	handler WatchLoopHandler

	// mtx guards below fields
	mtx sync.Mutex
	// wake can be called to force re-scan
	// may be nil
	wake func()
}

// WatchLoopHandler is the callback function for the WatchLoop.
// le may be nil
type WatchLoopHandler = func(
	ctx context.Context,
	le *logrus.Entry,
	world world.WorldState,
	obj world.ObjectState, // may be nil if not found or objkey is empty
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error)

// NewWatchLoop constructs a new Control Loop which looks up an Engine on the
// Bus and calls the Callback when the state changes.
//
// objectKey may be empty
// le may be nil
func NewWatchLoop(
	le *logrus.Entry,
	objectKey string,
	handler WatchLoopHandler,
) *WatchLoop {
	return &WatchLoop{
		le:        le,
		objectKey: objectKey,
		handler:   handler,
	}
}

// NewBusWatchLoop constructs a new BusEngine which attaches to an engine
// running on a controller bus.
func NewBusWatchLoop(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engineID string, write bool,
	objectKey string, handler WatchLoopHandler,
) (*WatchLoop, *world.BusEngine, world.WorldState) {
	busEngine := world.NewBusEngine(ctx, b, engineID)
	ws := world.NewEngineWorldState(ctx, busEngine, true)
	return NewWatchLoop(le, objectKey, handler), busEngine, ws
}

// ExecuteBusWatchLoop executes an existing WatchLoop with a Bus engine.
func ExecuteBusWatchLoop(
	ctx context.Context,
	b bus.Bus,
	engineID string, write bool,
	objLoop *WatchLoop,
) error {
	busEngine := world.NewBusEngine(ctx, b, engineID)
	defer busEngine.Close()
	ws := world.NewEngineWorldState(ctx, busEngine, true)
	return objLoop.Execute(ctx, ws)
}

// Wake forces the control loop to re-process the latest object state.
func (c *WatchLoop) Wake() {
	c.mtx.Lock()
	if wake := c.wake; wake != nil {
		wake()
		c.wake = nil
	}
	c.mtx.Unlock()
}

// Execute runs the ControlLoop execution loop.
func (c *WatchLoop) Execute(ctx context.Context, ws world.WorldState) error {
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

		var objState world.ObjectState
		var objFound bool
		if c.objectKey != "" {
			var err error
			objState, objFound, err = ws.GetObject(c.objectKey)
			if err != nil {
				return err
			}
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
		if err != nil && c.le != nil && err != context.Canceled {
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
