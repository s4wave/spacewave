package task_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

// inputObjectTracker tracks an input WorldObject for the Task.
type inputObjectTracker struct {
	// c is the controller
	c *Controller
	// objKey is the object key
	objKey string
	// objLoop tracks the object changes
	objLoop *world_control.ObjectLoop
	// firstCheck indicates this is the first check of the state.
	firstCheck bool
	// prevObjRev is the previous object revision.
	prevObjRev uint64
}

// newInputObjectTracker constructs a new input object tracker routine.
func (c *Controller) newInputObjectTracker(key string) (keyed.Routine, *inputObjectTracker) {
	tr := &inputObjectTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewObjectLoop(
		c.le.WithField("object-loop", "input-object-tracker"),
		key,
		tr.processState,
	)
	return tr.execute, tr
}

// execute executes the pass tracker.
func (t *inputObjectTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting input object tracker: %s", objKey)
	t.firstCheck = true
	return world_control.ExecuteBusObjectLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		false,
		t.objLoop,
	)
}

// processState processes the state for the pass.
func (t *inputObjectTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// skip the initial state (we saw it already)
	if t.firstCheck {
		t.firstCheck = false
		return true, nil
	}

	// if the object rev changed, trigger a re-check of the Task.
	if rev != t.prevObjRev {
		t.prevObjRev = rev
		le.Infof("input object changed: %s at %d", t.objKey, rev)
		t.c.objLoop.Wake()
	}

	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*inputObjectTracker)(nil)).processState
