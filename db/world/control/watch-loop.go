package world_control

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
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

	wake watchLoopWakeState
}

type watchLoopWakeState struct {
	// mtx guards below fields
	mtx sync.Mutex
	// waiter cancels the active world wait when one is registered.
	waiter *watchLoopWaiter
	// pending records a Wake call made while no world wait is registered.
	pending bool
}

type watchLoopWaiter struct {
	cancel context.CancelFunc
}

// WatchLoopHandler is the callback function for the WatchLoop.
//
// obj is borrowed for the duration of the call: the loop releases it before the
// next iteration. A handler that needs the object state afterwards acquires its
// own handle with world.WorldState.GetObject.
//
// le may be nil
type WatchLoopHandler = func(
	ctx context.Context,
	le *logrus.Entry,
	world world.WorldState,
	obj world.ObjectState, // may be nil if not found or objkey is empty
	rootRef *bucket.ObjectRef, // may be nil if not found or objkey is empty
	objRev uint64, // may be nil if not found or objkey is empty
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
	ws := world.NewEngineWorldState(busEngine, true)
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
	ws := world.NewEngineWorldState(busEngine, true)
	err := objLoop.Execute(ctx, ws)
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// Wake forces the control loop to re-process the latest object state.
func (c *WatchLoop) Wake() {
	c.wake.Wake()
}

func (w *watchLoopWakeState) Wake() {
	w.mtx.Lock()
	if waiter := w.waiter; waiter != nil {
		waiter.cancel()
		w.waiter = nil
		w.mtx.Unlock()
		return
	}
	w.pending = true
	w.mtx.Unlock()
}

func (w *watchLoopWakeState) beginWait(ctx context.Context) (context.Context, func(), bool) {
	waitCtx, cancel := context.WithCancel(ctx)
	w.mtx.Lock()
	if w.pending {
		w.pending = false
		w.mtx.Unlock()
		cancel()
		return waitCtx, func() {}, true
	}
	waiter := &watchLoopWaiter{cancel: cancel}
	w.waiter = waiter
	w.mtx.Unlock()
	return waitCtx, func() {
		cancel()
		w.mtx.Lock()
		if w.waiter == waiter {
			w.waiter = nil
		}
		w.mtx.Unlock()
	}, false
}

// Execute runs the ControlLoop execution loop.
func (c *WatchLoop) Execute(ctx context.Context, ws world.WorldState) error {
	if c == nil || c.handler == nil {
		return nil
	}

	for {
		done, err := c.executeOnce(ctx, ws)
		if done {
			return err
		}
	}
}

// executeOnce runs one iteration: it reads the watched object, calls the
// handler, then waits for the next revision. done reports that the loop is
// finished and Execute should return err.
//
// The object-state handle lives for exactly one iteration. A remote world state
// allocates a server-side resource per GetObject, so a loop that watches a
// long-running object would otherwise accumulate one handle per revision for as
// long as it runs.
func (c *WatchLoop) executeOnce(ctx context.Context, ws world.WorldState) (bool, error) {
	var rootRef *bucket.ObjectRef
	var rev uint64

	if ctx.Err() != nil {
		return true, context.Canceled
	}

	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		return true, err
	}

	var objState world.ObjectState
	var objFound bool
	if c.objectKey != "" {
		objState, objFound, err = ws.GetObject(ctx, c.objectKey)
		if err != nil {
			return true, err
		}
		defer world.ReleaseObjectState(objState)
	}
	if objFound {
		rootRef, rev, err = objState.GetRootRef(ctx)
		if err != nil {
			return true, err
		}
		if c.le != nil {
			c.le.
				WithField("object-key", c.objectKey).
				Debugf("object found at rev %d", rev)
		}
	} else {
		objState = nil
	}

	waitForChanges, err := c.handler(
		ctx, c.le,
		ws, objState,
		rootRef, rev,
	)
	if errors.Is(err, world.ErrUnhandledOp) {
		if c.le != nil {
			c.le.Debug("handler skipped unhandled operation")
		}
		waitForChanges = true
		err = nil
	} else if err != nil && c.le != nil &&
		(ctx.Err() == nil || !errors.Is(err, context.Canceled)) {
		le := c.le.WithError(err)
		if c.objectKey != "" {
			le = le.WithField("object-key", c.objectKey)
		}
		le.
			WithField("world-seqno", seqno).
			WithField("wait-for-changes", waitForChanges).
			Warn("handler returned error")
	}
	if !waitForChanges {
		return true, err
	}

	wakeCtx, finishWake, skipWait := c.wake.beginWait(ctx)
	if skipWait {
		return false, nil
	}

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
	finishWake()
	if err != nil && err != context.Canceled {
		return true, err
	}
	return false, nil
}
