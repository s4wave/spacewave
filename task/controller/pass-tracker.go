package task_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/keyed"
	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// passTracker tracks the latest Pass of the Task.
type passTracker struct {
	// c is the controller
	c *Controller
	// objKey is the object key
	objKey string
	// objLoop tracks the object changes
	objLoop *world_control.ObjectLoop
	// prevState is the previous pass state
	prevState *forge_pass.Pass
}

// newPassTracker constructs a new pass tracker routine.
func (c *Controller) newPassTracker(key string) keyed.Routine {
	tr := &passTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewObjectLoop(
		c.le.WithField("object-loop", "pass-tracker"),
		key,
		tr.processState,
	)
	return tr.execute
}

// execute executes the pass tracker.
func (t *passTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting pass tracker: %s", objKey)
	return world_control.ExecuteBusObjectLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		true,
		t.objLoop,
	)
}

// processState processes the state for the pass.
func (t *passTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := t.objKey

	// check the <type> of the object
	typesState := world_types.NewTypesState(ctx, ws)
	objType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return true, err
	}
	if objType != forge_pass.PassTypeID {
		return true, errors.Errorf("ignoring object with incorrect type: expected pass but got %s", objType)
	}

	passObj, _, err := forge_pass.LookupPass(ctx, ws, t.objKey)
	if err != nil {
		if err == context.Canceled {
			return true, nil
		}
		return true, errors.Wrap(err, "lookup pass")
	}

	if proto.Equal(passObj, t.prevState) {
		// no changes
		return true, nil
	}

	t.prevState = passObj
	switch passObj.GetPassState() {
	case forge_pass.State_PassState_COMPLETE:
	case forge_pass.State_PassState_UNKNOWN:
	default:
		return true, nil
	}

	// submit a transaction to update the Task with any changes to the Pass
	// this only can be submitted while the Task is RUNNING
	if err := t.c.updateWithPassState(ctx); err != nil {
		return true, err
	}

	return true, nil
}

// _ is a type assertion
var (
	_ keyed.Constructor               = ((*Controller)(nil)).newPassTracker
	_ world_control.ObjectLoopHandler = ((*passTracker)(nil)).processState
)
