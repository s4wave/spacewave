package task_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	task_transaction "github.com/aperturerobotics/forge/task/tx"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/task"

// Controller implements the Task controller.
// An Task is an attempt to process a given Target with Pass.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the execution controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// objKey is the object key (from the config)
	objKey string
	// peerID is the parsed peer id
	peerID peer.ID
	// peerIDStr is the parsed peer id string
	peerIDStr string
	// objLoop is the object watcher loop
	// watches the task object
	objLoop *world_control.WatchLoop
	// passWatcher manages watching the latest task Pass
	// the key is the object key of the pass
	passWatcher *keyed.Keyed[string, *passTracker]
	// inputObjectWatcher manages watching any input world objects.
	// the key is the object key of the input world object.
	inputObjectWatcher *keyed.Keyed[string, *inputObjectTracker]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:        le,
		bus:       bus,
		conf:      conf,
		objKey:    conf.GetObjectKey(),
		peerID:    peerID,
		peerIDStr: peerID.Pretty(),
	}
	c.passWatcher = keyed.NewKeyedWithLogger(c.newPassTracker, le)
	c.inputObjectWatcher = keyed.NewKeyedWithLogger(c.newInputObjectTracker, le)
	c.objLoop = world_control.NewWatchLoop(
		le.WithField("control-loop", "task-controller"),
		c.objKey,
		c.ProcessState,
	)
	return c
}

// StartControllerWithConfig starts a controller with a config.
// Waits for the controller to start.
// Returns a Release function to close the controller when done.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	ctrli, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	cl, ok := ctrli.(*Controller)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return cl, ctrlRef, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"task controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	c.passWatcher.SetContext(ctx, true)
	c.inputObjectWatcher.SetContext(ctx, true)
	return world_control.ExecuteBusWatchLoop(ctx, c.bus, c.conf.GetEngineId(), true, c.objLoop)
}

// updateWithPassState submits a transaction to update the Task with the latest Pass state.
func (c *Controller) updateWithPassState(ctx context.Context) error {
	// submit transaction to synchronize pass state
	busEngine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	wtx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer wtx.Discard()

	// lookup the task and make sure it is still in RUNNING state
	// ... and peer id matches
	taskObjKey := c.objKey
	taskObj, err := forge_task.LookupTask(ctx, wtx, taskObjKey)
	if err != nil {
		return errors.Wrap(err, "lookup task")
	}
	if taskObj.GetTaskState() != forge_task.State_TaskState_RUNNING {
		// task is not currently running, we have nothing to do.
		return nil
	}

	txd := task_transaction.NewTxUpdateWithPassState(c.objKey)
	_, _, err = wtx.ApplyWorldOp(ctx, txd, c.peerID)
	if err != nil {
		wtx.Discard()
	} else {
		err = wtx.Commit(ctx)
	}
	if err != nil && err != context.Canceled {
		if !errors.Is(err, forge_task.ErrUnknownState) {
			c.le.WithError(err).Warn("unable to update execution states")
		}
	}

	return err
}

// syncWatchPassStates starts/stop routines to watch the latest Pass state.
func (c *Controller) syncWatchPassStates(latestState *passState) {
	// determine the pass object key to watch
	var objKeys []string
	if latestState != nil && latestState.objKey != "" {
		objKeys = append(objKeys, latestState.objKey)
	}
	c.passWatcher.SyncKeys(objKeys, true)
}

// syncWatchInputObjects starts/stop routines to watch the task input objects.
func (c *Controller) syncWatchInputObjects(inputs []*forge_target.Input, watchAll bool) {
	var watchInputWorldObjects []string
	for _, tgtInput := range inputs {
		if !watchAll && !tgtInput.GetWatchChanges() {
			continue
		}
		if tgtInput.GetInputType() == forge_target.InputType_InputType_WORLD_OBJECT {
			tgtInputObjKey := tgtInput.GetWorldObject().GetObjectKey()
			if tgtInputObjKey != "" {
				watchInputWorldObjects = append(watchInputWorldObjects, tgtInputObjKey)
			}
		}
	}
	c.inputObjectWatcher.SyncKeys(watchInputWorldObjects, true)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
