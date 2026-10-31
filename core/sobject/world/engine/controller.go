package sobject_world_engine

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// servedEngine is an engine the controller publishes. Holders keep mtx read
// locked; withdraw takes the write lock to retire the engine, so the engine
// closes only after every holder released it.
type servedEngine struct {
	// engine is the published engine.
	engine Engine
	// mtx is read locked by each holder of engine.
	mtx sync.RWMutex
	// retired is set once the engine is withdrawn. Guarded by mtx.
	retired bool
}

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
	engineCtr *ccontainer.CContainer[*servedEngine]
	// engineID identifies the engine served by lookup directives.
	engineID string

	// sfs constructs the World block transformers.
	sfs *block_transform.StepFactorySet
	// opTypesMu guards opTypes.
	opTypesMu sync.RWMutex
	// opTypes supplies built-in operations before bus lookup and the operation
	// types the World declares.
	opTypes OpTypes

	// writeMtx serializes write transactions and replay of the watched
	// SharedObject state.
	writeMtx writerLock
	// writer is the last served engine, whose admissions lockSettled settles,
	// or nil before the first. Guarded by writeMtx.
	writer *soEngine
	// failed holds the local ids of the operations whose publications failed.
	// It outlives the engine, so a replacement skips the snapshots that still
	// show them. Guarded by writeMtx.
	failed map[string]struct{}
	// writeBcast is broadcast after each change of the installed World. The
	// storage reclaim routine schedules a pass after each broadcast.
	writeBcast broadcast.Broadcast
	// reinstallMtx guards reinstallHooks, which are called under the writer
	// lock with the World seqno after failed writes are dropped. They outlive
	// the engine.
	reinstallMtx   sync.Mutex
	reinstallHooks []*reinstallHook
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	sfs *block_transform.StepFactorySet,
) (*Controller, error) {
	le = le.WithField("engine-id", conf.GetEngineId())
	return &Controller{
		le:        le,
		conf:      conf,
		bus:       bus,
		engineCtr: ccontainer.NewCContainer[*servedEngine](nil),
		engineID:  conf.GetEngineId(),
		sfs:       sfs,
		writeMtx:  writerLock{le: le},
		failed:    make(map[string]struct{}),
	}, nil
}

// SetOpTypes sets how this engine resolves operation types. Its lookup is
// composed before the bus lookup, so domain-owned built-ins do not depend on
// an outer controller lifecycle.
func (c *Controller) SetOpTypes(opTypes OpTypes) {
	c.opTypesMu.Lock()
	c.opTypes = opTypes
	c.opTypesMu.Unlock()
}

// getOpTypes returns how this engine resolves operation types.
func (c *Controller) getOpTypes() OpTypes {
	c.opTypesMu.RLock()
	defer c.opTypesMu.RUnlock()
	return c.opTypes
}

// buildLookupWorldOp composes the current built-in operations with bus lookup.
func (c *Controller) buildLookupWorldOp(le *logrus.Entry) world.LookupOp {
	var busLookupOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		busLookupOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}
	return func(ctx context.Context, operationTypeID string) (world.Operation, error) {
		// Try the built-in operations first, then the bus.
		var lookupOps []world.LookupOp
		if lookupOp := c.getOpTypes().Lookup; lookupOp != nil {
			lookupOps = append(lookupOps, lookupOp)
		}
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
	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Mount the shared object and serve it.
	c.le.Debug("mounting shared object")
	so, soRef, err := sobject.ExMountSharedObject(rctx, c.bus, c.conf.GetRef(), false, rctxCancel)
	if err != nil {
		return err
	}
	defer soRef.Release()
	return c.executeSharedObject(rctx, rctxCancel, so)
}

// executeSharedObject serves the World of the mounted so until rctx ends or an
// error ends it. rctxCancel cancels rctx when so or its state is released.
func (c *Controller) executeSharedObject(rctx context.Context, rctxCancel context.CancelFunc, so sobject.SharedObject) error {
	// Hold the World engine lease while serving.
	le := c.le
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

	// Access the state the engine replays.
	soStateCtr, soStateCtrRel, err := accessAdmittedState(rctx, so, rctxCancel)
	if err != nil {
		return err
	}
	defer soStateCtrRel()

	// Acknowledge the edits this device holds and keep the trimming roster
	// while the shared object is mounted. A device whose replay stalls on a
	// missing block still acknowledges, so the checkpointer can checkpoint
	// past the stall and the device can resume from that checkpoint.
	go func() {
		if err := sobject.Acknowledge(rctx, so, c.acknowledger(so)); err != nil && rctx.Err() == nil {
			le.WithError(err).Warn("stopped acknowledging edits")
		}
	}()
	if roster, ok := so.(sobject.RosterHost); ok {
		go func() {
			if err := sobject.MaintainRoster(rctx, so, roster); err != nil && rctx.Err() == nil {
				le.WithError(err).Warn("stopped maintaining the trimming roster")
			}
		}()
	}

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
	// Wait for the writes the previous engine admitted to settle, so this
	// engine starts from the store and knows which operations failed.
	unlock, err := c.lockSettled(ctx, "execute-world")
	if err != nil {
		return err
	}
	unlock()

	// Resume the saved replay, then replay the World, initializing it if
	// necessary.
	le := c.le
	replay := newReplayer(c, so)
	replay.pages = c.newWorldPages(so)
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

	// Bind the head to the engine's view of the block store, once for
	// readers and once for writers, which fork from the admitted World ahead
	// of the durable one. Only writers commit, so only they gather what each
	// World commit replaces from here on.
	blkEngine, err := c.buildBlkEngine(ctx, le, replay.store(), headRef, transformConf)
	if err != nil {
		return err
	}
	defer blkEngine.Release()
	replay.deaths = newWorldDeaths(replay.storedRoot(headRef.GetRootRef()))
	admitted, err := c.buildBlkEngine(ctx, le, replay.store(), headRef, transformConf, replay.transitions()...)
	if err != nil {
		return err
	}
	defer admitted.Release()
	blkEngine.admitted = admitted.bengine

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

	// The engine's commits and background routines end with this call, which
	// can return before ctx ends when read access is lost and a new engine
	// replaces this one.
	bgCtx, bgCancel := context.WithCancel(ctx)
	defer bgCancel()

	// Wrap the world block engine with our txn logic for sobject.
	engine := newSoEngine(bgCtx, c, so, blkEngine, replay)
	if host, ok := so.(sobject.StateHost); ok {
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
	if err := c.setWriter(ctx, engine, seqno); err != nil {
		return err
	}
	served := &servedEngine{engine: wengine}
	c.engineCtr.SetValue(served)
	defer c.withdraw(served)

	// Order the Space as its main device, vote in the group's decisions and
	// reclaim storage while the World is served.
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

// acknowledger returns the acknowledgment write for so: it queues an empty
// operation after every write transaction this device has started.
func (c *Controller) acknowledger(so sobject.SharedObject) func(context.Context) error {
	return func(ctx context.Context) error {
		// Queue it under the writer lock once the admitted writes settle, so
		// the direct state write does not wait for their receipts under it.
		unlockWriteMtx, err := c.lockSettled(ctx, "acknowledge")
		if err != nil {
			return err
		}
		defer unlockWriteMtx()
		_, err = so.QueueOperation(ctx, nil)
		return err
	}
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
	served, err := c.engineCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return served.engine, nil
}

// AcquireWorldEngine waits for the served engine and holds it open until
// release is called. The engine lookup resolver withdraws the engine before
// releasing it, so no lookup sees the engine close.
func (c *Controller) AcquireWorldEngine(ctx context.Context) (Engine, func(), error) {
	for {
		served, err := c.engineCtr.WaitValue(ctx, nil)
		if err != nil {
			return nil, nil, err
		}
		served.mtx.RLock()
		if !served.retired {
			return served.engine, served.mtx.RUnlock, nil
		}
		// withdraw cleared the container before retiring it, so the next
		// wait returns a successor.
		served.mtx.RUnlock()
	}
}

// WaitWorldEngineReplaced waits until eng is no longer the published engine.
// The controller replaces its engine when read access is lost and regained;
// the engine lookup resolver follows each replacement through this wait.
func (c *Controller) WaitWorldEngineReplaced(ctx context.Context, eng Engine) error {
	_, err := c.engineCtr.WaitValueWithValidator(ctx, func(current *servedEngine) (bool, error) {
		return current == nil || current.engine != eng, nil
	}, nil)
	return err
}

// withdraw stops publishing served and waits for every holder to release it.
// The caller closes the engine afterwards.
func (c *Controller) withdraw(served *servedEngine) {
	c.engineCtr.SetValue(nil)
	served.mtx.Lock()
	served.retired = true
	served.mtx.Unlock()
}

// setWriter sets the served engine under the writer lock and calls the
// reinstall hooks with the seqno of its head. executeWorld settled the
// admissions of the engine it replaces before reading that head, and a
// retired engine admits nothing, so no write of the replaced engine settles
// after the hooks see the new head.
func (c *Controller) setWriter(ctx context.Context, e *soEngine, seqno uint64) error {
	// Swap the writer under the writer lock and report its World to the
	// reinstall hooks, since writes of the engine before it may have failed.
	unlock, err := c.writeMtx.Lock(ctx, "set-writer")
	if err != nil {
		return err
	}
	c.writer = e
	c.callReinstall(seqno)
	unlock()
	return nil
}

// staleSnapshot reports whether snap of so shows a failed write, for a caller
// outside the writer lock.
func (c *Controller) staleSnapshot(ctx context.Context, so sobject.SharedObject, snap sobject.SharedObjectStateSnapshot) (bool, error) {
	unlock, err := c.writeMtx.Lock(ctx, "stale-snapshot")
	if err != nil {
		return false, err
	}
	defer unlock()
	return c.stale(ctx, so, snap)
}

// lockSettled locks the writer lock once no admission of the served engine
// waits for its receipt and returns the unlock function. A caller that writes
// the SharedObject state directly or holds the replayed Worlds itself uses it,
// since the direct write waits for every pending receipt and only a pending
// publication holds the blocks of its World.
func (c *Controller) lockSettled(ctx context.Context, holder string) (func(), error) {
	for {
		// Lock and settle the resolved receipts.
		unlock, err := c.writeMtx.Lock(ctx, holder)
		if err != nil {
			return nil, err
		}
		if c.writer == nil {
			return unlock, nil
		}
		c.writer.settle()
		settled := c.writer.settled()
		if settled == nil {
			return unlock, nil
		}

		// Wait outside the lock for the oldest receipt to resolve.
		unlock()
		select {
		case <-settled:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ world.ReplacingController = (*Controller)(nil)
