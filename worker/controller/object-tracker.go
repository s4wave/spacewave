package worker_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	cluster_controller "github.com/aperturerobotics/forge/cluster/controller"
	forge_execution "github.com/aperturerobotics/forge/execution"
	exec_controller "github.com/aperturerobotics/forge/execution/controller"
	forge_pass "github.com/aperturerobotics/forge/pass"
	pass_controller "github.com/aperturerobotics/forge/pass/controller"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	task_controller "github.com/aperturerobotics/forge/task/controller"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// objectTracker tracks a object managed by the Worker.
type objectTracker struct {
	// c is the controller
	c *Controller
	// objKey is the object key
	objKey string

	// objLoop tracks the object changes
	objLoop *world_control.WatchLoop
	// objTypeCtr is the object type ccontainer
	objTypeCtr *ccontainer.CContainer[string]

	// the following fields are modified by execute() only
	// ctrlCancel is the context cancel for the controller
	ctrlCancel context.CancelFunc
	// ctrlObjType is the current running object type
	ctrlObjType string
}

// newObjectTracker constructs a new object tracker routine.
func (c *Controller) newObjectTracker(key string) (keyed.Routine, *objectTracker) {
	tr := &objectTracker{
		c:          c,
		objKey:     key,
		objTypeCtr: ccontainer.NewCContainer(""),
	}
	tr.objLoop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "object-tracker"),
		key,
		tr.processState,
	)
	return tr.execute, tr
}

// execute executes the job tracker.
func (t *objectTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting object tracker: %s", objKey)
	errCh := make(chan error, 2)
	go func() {
		errCh <- world_control.ExecuteBusWatchLoop(
			ctx,
			t.c.bus,
			t.c.conf.GetEngineId(),
			true,
			t.objLoop,
		)
	}()

	var err error
	var prevVal string
	var objType string
	for {
		// Wait for the object type to be set and/or changed
		prevVal, err = t.objTypeCtr.WaitValueChange(ctx, prevVal, errCh)
		if err != nil {
			return err
		}
		if prevVal != "" {
			objType = prevVal
		} else {
			objType = ""
		}

		// Sync object type to the controller if needed
		if err := t.applyObjectType(ctx, objType); err != nil {
			t.c.le.WithError(err).Warn("unable to start object controller")
		}
	}
}

// applyObjectType is called by the execute() loop to apply the object type.
func (t *objectTracker) applyObjectType(ctx context.Context, objType string) error {
	if t.ctrlObjType == objType {
		return nil
	}
	if t.ctrlCancel != nil {
		t.ctrlCancel()
		t.ctrlCancel = nil
	}
	t.ctrlObjType = objType
	if objType == "" {
		return nil
	}

	// generate controller config for the object type
	ctrlConf, err := t.buildCtrlConf(ctx, objType)
	if err != nil {
		return err
	}
	if ctrlConf == nil {
		return nil
	}

	ctrlCtx, ctrlCancel := context.WithCancel(ctx)
	t.ctrlCancel = ctrlCancel
	go t.executeController(ctrlCtx, objType, ctrlConf)
	return nil
}

// buildCtrlConf builds the controller config for a given object type.
func (t *objectTracker) buildCtrlConf(ctx context.Context, objType string) (config.Config, error) {
	engineID := t.c.conf.GetEngineId()
	objKey := t.objKey
	peerID := t.c.peerID
	switch objType {
	case forge_cluster.ClusterTypeID:
		return cluster_controller.NewConfig(engineID, objKey, peerID), nil
	case forge_task.TaskTypeID:
		return task_controller.NewConfig(engineID, objKey, peerID, t.c.conf.GetAssignSelf()), nil
	case forge_pass.PassTypeID:
		return pass_controller.NewConfig(engineID, objKey, peerID, t.c.conf.GetAssignSelf()), nil
	case forge_execution.ExecutionTypeID:
		// TODO: where do we get the "target world" from?
		// clean up the "target world" concept
		return exec_controller.NewConfig(
			engineID,
			objKey,
			peerID,
			&forge_target.InputWorld{EngineId: engineID},
		), nil
	default:
		return nil, errors.Wrap(world_types.ErrUnknownObjectType, objType)
	}
}

// executeController applies the directive to execute the object controller.
// exits when ctx is canceled
func (t *objectTracker) executeController(ctx context.Context, objType string, ctrlConf config.Config) {
	t.c.le.
		WithField("config-id", ctrlConf.GetConfigID()).
		WithField("obj-type", objType).
		Debugf("starting controller for object: %s", t.objKey)
	_, diRef, err := t.c.bus.AddDirective(resolver.NewLoadControllerWithConfig(ctrlConf), nil)
	if err != nil {
		if err != context.Canceled {
			t.c.le.WithError(err).Warn("unable to start object controller")
		}
		return
	}
	<-ctx.Done()
	diRef.Release()
}

// processState processes the state for the job.
func (t *objectTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := t.objKey

	defer func() {
		if err != nil {
			t.pushObjType("")
		}
	}()

	// check the <type> of the object
	typesState := world_types.NewTypesState(ctx, ws)
	objType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return false, err
	}

	t.pushObjType(objType)
	return true, nil
}

// pushObjType pushes the object info from processState.
func (t *objectTracker) pushObjType(objType string) {
	if objType != "" {
		t.objTypeCtr.SetValue(objType)
	} else {
		t.objTypeCtr.SetValue("")
	}
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*objectTracker)(nil)).processState
