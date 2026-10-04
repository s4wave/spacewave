package sobject_world_engine

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// Controller drives a World Graph engine bound to a block graph and controlled by a Shared Object.
// Uses MountSharedObject to mount and access the shared object and block store.
// Every member replays the Shared Object operation set to the same World.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// bus resolves World dependencies and operations.
	bus bus.Bus
	// conf configures the mounted SharedObject and World engine.
	conf *Config
	// engineCtr publishes the ready engine for this controller execution.
	engineCtr *ccontainer.CContainer[*Engine]
	// engineID identifies the engine served by lookup directives.
	engineID string

	// sfs constructs the World block transformers.
	sfs *block_transform.StepFactorySet
	// staticLookupOpMu guards staticLookupOp.
	staticLookupOpMu sync.RWMutex
	// staticLookupOp supplies built-in operations before bus lookup.
	staticLookupOp world.LookupOp

	// writeMtx serializes write transactions and replay of the watched
	// SharedObject state.
	writeMtx csync.Mutex
	// writeBcast is broadcast after each change of the installed World. The
	// storage reclaim routine schedules a pass after each broadcast.
	writeBcast broadcast.Broadcast
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	sfs *block_transform.StepFactorySet,
) (*Controller, error) {
	return &Controller{
		le:        le.WithField("engine-id", conf.GetEngineId()),
		conf:      conf,
		bus:       bus,
		engineCtr: ccontainer.NewCContainer[*Engine](nil),
		engineID:  conf.GetEngineId(),
		sfs:       sfs,
	}, nil
}

// SetStaticLookupOp sets an in-process world operation lookup chain for this
// engine. It is composed before the bus lookup, so domain-owned built-ins do
// not depend on an outer controller lifecycle.
func (c *Controller) SetStaticLookupOp(lookupOp world.LookupOp) {
	c.staticLookupOpMu.Lock()
	c.staticLookupOp = lookupOp
	c.staticLookupOpMu.Unlock()
}

// buildLookupWorldOp composes the current built-in operations with bus lookup.
func (c *Controller) buildLookupWorldOp(le *logrus.Entry) world.LookupOp {
	var busLookupOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		busLookupOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}
	return func(ctx context.Context, operationTypeID string) (world.Operation, error) {
		// Collect the static and bus lookups.
		var lookupOps []world.LookupOp
		c.staticLookupOpMu.RLock()
		if c.staticLookupOp != nil {
			lookupOps = append(lookupOps, c.staticLookupOp)
		}
		c.staticLookupOpMu.RUnlock()
		if busLookupOp != nil {
			lookupOps = append(lookupOps, busLookupOp)
		}
		return world.LookupOpSlice(lookupOps).LookupOp(ctx, operationTypeID)
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"block world engine controller: "+c.engineID,
	)
}

// Execute executes the engine controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Scope the routines to this execution.
	le := c.le
	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Mount the shared object.
	le.Debug("mounting shared object")
	so, soRef, err := sobject.ExMountSharedObject(rctx, c.bus, c.conf.GetRef(), false, rctxCancel)
	if err != nil {
		return err
	}
	defer soRef.Release()
	worldEngineLease, detectsLeaseLoss, err := c.acquireWorldEngineLease(rctx, so)
	if err != nil {
		return err
	}
	if worldEngineLease != nil {
		watchWorldEngineLease(rctx, worldEngineLease, detectsLeaseLoss, rctxCancel)
		defer func() {
			if err := worldEngineLease.Release(rctx); err != nil {
				le.WithError(err).Warn("failed to release World Engine lease")
			}
		}()
	}

	// access the shared object state
	soStateCtr, soStateCtrRel, err := so.AccessSharedObjectState(rctx, rctxCancel)
	if err != nil {
		return err
	}
	defer soStateCtrRel()

	// Serve the World while this participant can replay it. A participant
	// that loses read access waits for readmission or a missing grant here:
	// restarting through the controller backoff would report the stale denial
	// to body mounts made after readmission.
	for {
		err := c.executeWorld(rctx, so, soStateCtr)
		if !isReadAccessLoss(err) {
			return err
		}
		le.WithError(err).Debug("waiting for read access to the shared object")
		if err := waitReadableSnapshot(rctx, soStateCtr); err != nil {
			return err
		}
	}
}

// executeWorld replays the World, publishes its engine, and follows the
// operation set until an error ends it.
func (c *Controller) executeWorld(
	ctx context.Context,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) error {
	// Resume the saved replay, then replay the World, initializing it if
	// necessary.
	le := c.le
	replay := newReplayer(c, so)
	if err := replay.load(ctx); err != nil {
		return err
	}
	headState, err := c.waitWorldInit(ctx, so, soStateCtr, replay)
	if err != nil {
		return err
	}

	// Blocks are unreadable without the head's transform configuration.
	headRef := headState.GetHeadRef()
	transformConf := headRef.GetTransformConf()
	if len(transformConf.GetSteps()) == 0 {
		return sobject.ErrEmptyTransformConfig
	}

	// Bind the head to the block store.
	blkEngine, err := c.buildBlkEngine(ctx, le, so, headRef, transformConf)
	if err != nil {
		return err
	}
	defer blkEngine.Release()

	// Log the bound root and read the World sequence number.
	if c.conf.GetVerbose() {
		le.
			WithField("world-root", headRef.MarshalB58()).
			Debug("initialized world root")
	}
	seqno, err := blkEngine.bengine.GetSeqno(ctx)
	if err != nil {
		return err
	}

	// Wrap the world block engine with our txn logic for sobject.
	engine := newSoEngine(c, so, blkEngine.bengine, replay)
	if host, ok := so.(sobject.InviteHost); ok {
		engine.control, err = sobject.NewControl(le, host.GetSOHost(), host.GetPrivKey(), engine)
		if err != nil {
			return err
		}
	}

	// Load this device's backfill choice before readers can watch it.
	backfill, err := readBackfill(ctx, so)
	if err != nil {
		return err
	}
	engine.backfillChoice.SetValue(backfill)

	// Wrap the engine with logging when verbose.
	var wengine world.Engine = engine
	if c.conf.GetVerbose() {
		wengine = world_vlogger.NewEngine(le, wengine)
	}

	// Publish the engine for the lifetime of this World.
	le.WithField("world-seqno", seqno).Info("world engine ready")
	c.engineCtr.SetValue(&wengine)
	defer c.engineCtr.SetValue(nil)

	// Acknowledge the edits this device builds on and restore returning
	// devices to the trimming roster while it serves the World.
	bgCtx, bgCancel := context.WithCancel(ctx)
	defer bgCancel()
	go func() {
		if err := sobject.Acknowledge(bgCtx, so, engine.acknowledge); err != nil && bgCtx.Err() == nil {
			le.WithError(err).Warn("stopped acknowledging edits")
		}
	}()
	if roster, ok := so.(sobject.RosterHost); ok {
		go func() {
			if err := sobject.RestoreRoster(bgCtx, so, roster); err != nil && bgCtx.Err() == nil {
				le.WithError(err).Warn("stopped restoring returning devices")
			}
		}()
	}

	// Order the Space as its main device, vote in the group's decisions and
	// reclaim storage.
	if mainDevice, ok := so.(sobject.MainDevice); ok {
		go func() {
			if err := sobject.Sequence(bgCtx, so, mainDevice.SequenceOperations); err != nil && bgCtx.Err() == nil {
				le.WithError(err).Warn("stopped ordering edits as the main device")
			}
		}()
	}
	if engine.control != nil {
		go func() {
			if err := engine.control.Execute(bgCtx); err != nil && bgCtx.Err() == nil {
				le.WithError(err).Warn("stopped voting in group decisions")
			}
		}()
	}
	go func() {
		if err := c.executeStorageReclaim(bgCtx, engine); err != nil && bgCtx.Err() == nil {
			le.WithError(err).Warn("stopped reclaiming storage")
		}
	}()

	// Backfill the World when this device chose to.
	engine.backfill.SetContext(bgCtx, false)
	engine.backfill.SetState(engine.backfillChoice.GetValue())

	// Follow the operation set into the World.
	return c.executeWatchSOState(ctx, soStateCtr, engine)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	// LookupWorldEngine handler.
	if d, ok := dir.(world.LookupWorldEngine); ok {
		return directive.R(c.resolveLookupWorldEngine(ctx, di, d))
	}

	return nil, nil
}

// GetWorldEngine waits for the engine to be built.
// Returns the Engine managed by the controller.
func (c *Controller) GetWorldEngine(ctx context.Context) (Engine, error) {
	val, err := c.engineCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// WaitWorldEngineReplaced waits until eng is no longer the published engine.
// The controller replaces its engine when read access is lost and regained.
func (c *Controller) WaitWorldEngineReplaced(ctx context.Context, eng Engine) error {
	_, err := c.engineCtr.WaitValueWithValidator(ctx, func(current *Engine) (bool, error) {
		return current == nil || *current != eng, nil
	}, nil)
	return err
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ world.Controller = (*Controller)(nil)
