package flowgraph_run

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/sirupsen/logrus"
)

// taskWatcher wakes the run loop on each revision of one activation's Task.
type taskWatcher struct {
	// c is the run controller.
	c *Controller
	// loop watches the Task object.
	loop *world_control.WatchLoop
}

// newTaskWatcher constructs the watcher routine for a Task key.
func (c *Controller) newTaskWatcher(key string) (keyed.Routine, *taskWatcher) {
	w := &taskWatcher{c: c}
	w.loop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "flowgraph-run-task"),
		key,
		w.processState,
	)
	return w.execute, w
}

// execute watches the Task until the run no longer waits on it.
func (w *taskWatcher) execute(ctx context.Context) error {
	return w.loop.Execute(ctx, world.NewEngineWorldState(w.c.engine, false))
}

// processState wakes the run loop, which reads the Task in its transaction.
func (w *taskWatcher) processState(
	context.Context,
	*logrus.Entry,
	world.WorldState,
	world.ObjectState,
	*bucket.ObjectRef,
	uint64,
) (bool, error) {
	w.c.loop.Wake()
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*taskWatcher)(nil).processState
