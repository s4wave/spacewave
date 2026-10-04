package worker_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	exec_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/sirupsen/logrus"
)

// objectTracker watches a managed object and retains its controller demand.
type objectTracker struct {
	// c is the Worker controller that retains this tracker.
	c *Controller
	// objKey identifies the managed object.
	objKey string

	// objLoop watches managed-object revisions.
	objLoop *world_control.WatchLoop
	// objTypeCtr publishes the object type this Worker can execute.
	objTypeCtr *ccontainer.CContainer[string]

	// ctrlCancel stops the controller demand; only execute modifies it.
	ctrlCancel context.CancelFunc
	// ctrlObjType is the running type; only execute modifies it.
	ctrlObjType string
}

// newObjectTracker constructs a new object tracker routine.
func (c *Controller) newObjectTracker(key string) (keyed.Routine, *objectTracker) {
	// Create the tracker and its object-type notification.
	tr := &objectTracker{
		c:          c,
		objKey:     key,
		objTypeCtr: ccontainer.NewCContainer(""),
	}

	// Watch object revisions through the Worker's World engine.
	tr.objLoop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "object-tracker"),
		key,
		tr.processAssignedState,
	)
	return tr.execute, tr
}

// execute executes the job tracker.
func (t *objectTracker) execute(ctx context.Context) error {
	// Run the World revision watch for this tracker's lifetime.
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

	// Reconcile controller demand whenever the executable object type changes.
	var err error
	var prevVal string
	var objType string
	for {
		// Observe the next executable object type.
		prevVal, err = t.objTypeCtr.WaitValueChange(ctx, prevVal, errCh)
		if err != nil {
			return err
		}
		objType = prevVal

		// Reconcile the controller demand to the observed type.
		if err := t.applyObjectType(ctx, objType); err != nil {
			if ctx.Err() == nil || !errors.Is(err, context.Canceled) {
				t.c.le.WithError(err).Warn("unable to start object controller")
			}
		}
	}
}

// applyObjectType is called by the execute() loop to apply the object type.
func (t *objectTracker) applyObjectType(ctx context.Context, objType string) error {
	// Replace the prior controller demand only when its object type changes.
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

	// Resolve the controller configuration for the new object type.
	ctrlConf, err := t.buildCtrlConf(ctx, objType)
	if err != nil {
		return err
	}
	if ctrlConf == nil {
		return nil
	}

	// Retain the new controller demand until this tracker stops.
	ctrlCtx, ctrlCancel := context.WithCancel(ctx)
	t.ctrlCancel = ctrlCancel
	go t.executeController(ctrlCtx, objType, ctrlConf)
	return nil
}

// buildCtrlConf builds the controller config for a given object type.
func (t *objectTracker) buildCtrlConf(ctx context.Context, objType string) (config.Config, error) {
	// Select the controller for the managed object's Forge type.
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
		conf := exec_controller.NewConfig(
			engineID,
			objKey,
			peerID,
			&forge_target.InputWorld{EngineId: engineID},
		)
		conf.WorkerObjectKey = t.c.objKey
		conf.ClaimId = conf.BuildUniqueID()
		return conf, nil
	case forge_worker.WorkerTypeID:
		// The owning WorkerController already manages its worker object.
		return nil, nil
	default:
		return nil, errors.Wrap(world_types.ErrUnknownObjectType, objType)
	}
}

// executeController applies the directive to execute the object controller.
// exits when ctx is canceled
func (t *objectTracker) executeController(ctx context.Context, objType string, ctrlConf config.Config) {
	// Retain the directive that runs the managed object's controller.
	t.c.le.
		WithField("config-id", ctrlConf.GetConfigID()).
		WithField("obj-type", objType).
		Debugf("starting controller for object: %s", t.objKey)
	_, diRef, err := t.c.bus.AddDirective(resolver.NewLoadControllerWithConfig(ctrlConf), nil)
	if err != nil {
		if ctx.Err() == nil || !errors.Is(err, context.Canceled) {
			t.c.le.WithError(err).Warn("unable to start object controller")
		}
		return
	}
	defer diRef.Release()

	// Release controller demand when the tracker lifetime ends.
	<-ctx.Done()
}

// processState processes the state for the job.
func (t *objectTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// Clear the running controller when reading or binding custody fails.
	objKey := t.objKey
	defer func() {
		if err != nil {
			t.pushObjType("")
		}
	}()

	// Read the managed object type before choosing its controller.
	objType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return false, err
	}

	// Bind automatic placement before an Execution controller can start its target.
	if objType == forge_execution.ExecutionTypeID {
		engine := world.NewBusEngine(ctx, t.c.bus, t.c.conf.GetEngineId())
		var assigned bool
		err = world.ExecTransaction(ctx, engine, true, func(ctx context.Context, tx world.WorldState) error {
			var err error
			assigned, err = forge_execution.BindExecutionWorker(ctx, tx, objKey, &forge_worker.Placement{
				WorkerObjectKey: t.c.objKey,
				PeerId:          t.c.peerID.String(),
			})
			return err
		})
		if err != nil {
			return false, err
		}
		if !assigned {
			// Observe an active foreign claim so this Worker can recover it after
			// expiry. Pending placement remains exclusive to its selected Worker.
			execution, err := world.LookupObjectBody[*forge_execution.Execution](ctx, ws, objKey, forge_execution.NewExecutionBlock)
			if err != nil {
				return false, err
			}
			if execution.GetClaim() == nil || execution.IsComplete() {
				t.pushObjType("")
				return true, nil
			}
		}
	}

	// Publish only objects this Worker can execute.
	t.pushObjType(objType)
	return true, nil
}

// processAssignedState releases retired objects after their final result is
// durable, and retains ordinary controller demand for live work.
func (t *objectTracker) processAssignedState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState,
	rootRef *bucket.ObjectRef, rev uint64,
) (bool, error) {
	// Reconcile retirement before choosing the object's executable controller.
	retired, err := forge_task.ReconcileRetiredObject(ctx, ws, t.objKey)
	if err != nil {
		return false, err
	}
	if retired {
		t.pushObjType("")
		return false, nil
	}

	// Preserve normal reactive demand until one-shot work is fully settled.
	return t.processState(ctx, le, ws, obj, rootRef, rev)
}

// pushObjType pushes the object info from processState.
func (t *objectTracker) pushObjType(objType string) {
	t.objTypeCtr.SetValue(objType)
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*objectTracker)(nil).processState
