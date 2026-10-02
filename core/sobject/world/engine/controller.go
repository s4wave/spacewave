package sobject_world_engine

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// Controller drives a World Graph engine bound to a block graph and controlled by a Shared Object.
// Uses MountSharedObject to mount and access the shared object and block store.
// Stores the HEAD reference in the Shared Object.
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

	// processOpsAsValidator is the routine to process incoming operations as a validator.
	processOpsAsValidator *routine.RoutineContainer

	// sfs constructs the World block transformers.
	sfs *block_transform.StepFactorySet
	// staticLookupOpMu guards staticLookupOp.
	staticLookupOpMu sync.RWMutex
	// staticLookupOp supplies built-in operations before bus lookup.
	staticLookupOp world.LookupOp

	// writeMtx guards write transactions / updating local state due to watching SOState.
	// only one of the two activities will be active at a time.
	writeMtx csync.Mutex

	// retainMtx serializes retainRoots, which owns the retained roots' proof
	// store and local named root.
	retainMtx sync.Mutex

	// lastCommitResult caches the latest foreground commit for replay adoption.
	// Written during foreground writes under writeMtx and read by the
	// validator without writeMtx.
	lastCommitResult atomic.Pointer[commitResult]
}

// commitResult caches a foreground commit result for replay adoption.
// Replay consumers can adopt this result when the base root ref and op bytes
// match, avoiding expensive re-execution of processOp.
// It is immutable once published to the validator.
type commitResult struct {
	// baseRootRef identifies the accepted World used to compute the candidate.
	baseRootRef *block.BlockRef
	// opData is the exact encoded operation used to compute the candidate.
	opData []byte
	// resultRef is the candidate World head.
	resultRef *bucket.ObjectRef
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	sfs *block_transform.StepFactorySet,
) (*Controller, error) {
	processBackoff := conf.GetProcessOpsBackoff()
	if processBackoff == nil {
		processBackoff = &backoff.Backoff{BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL}
	}

	return &Controller{
		le:        le.WithField("engine-id", conf.GetEngineId()),
		conf:      conf,
		bus:       bus,
		engineCtr: ccontainer.NewCContainer[*Engine](nil),
		engineID:  conf.GetEngineId(),

		processOpsAsValidator: routine.NewRoutineContainer(
			routine.WithExitLogger(le),
			routine.WithRetry(processBackoff),
		),

		sfs: sfs,
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

	// start the process ops routine (as a validator)
	_, _ = c.processOpsAsValidator.SetRoutine(func(ctx context.Context) error {
		return c.executeProcessOpsWhenValidator(ctx, so, soStateCtr)
	})
	_ = c.processOpsAsValidator.SetContext(rctx, true)
	defer c.processOpsAsValidator.ClearContext()

	// Serve the World while this participant can read it. A participant that
	// loses read access waits for readmission here: restarting through the
	// controller backoff would report the stale denial to body mounts made
	// after readmission.
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

// executeWorld builds the World from the accepted head, publishes its engine,
// and follows accepted state until an error ends it.
func (c *Controller) executeWorld(
	ctx context.Context,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) error {
	// Initialize the shared object if necessary.
	le := c.le
	headState, err := c.loadOrInitHeadFromSharedObject(ctx, so, soStateCtr)
	if err != nil {
		return err
	}

	// The World bucket is the SharedObject block store.
	if headState.HeadRef == nil {
		headState.HeadRef = &bucket.ObjectRef{}
	}
	headState.HeadRef.BucketId = so.GetBlockStore().GetID()

	// Blocks are unreadable without the head's transform configuration.
	transformConf := headState.HeadRef.GetTransformConf()
	if len(transformConf.GetSteps()) == 0 {
		return sobject.ErrEmptyTransformConfig
	}

	// Bind the head to the block store.
	blkEngine, err := c.buildBlkEngine(ctx, le, so, headState.HeadRef, transformConf)
	if err != nil {
		return err
	}
	defer blkEngine.Release()

	// Log the bound root and read the World sequence number.
	if c.conf.GetVerbose() {
		le.
			WithField("world-root", headState.HeadRef.MarshalB58()).
			Debug("initialized world root")
	}
	seqno, err := blkEngine.bengine.GetSeqno(ctx)
	if err != nil {
		return err
	}

	// Wrap the world block engine with our txn logic for sobject.
	engine := newSoEngine(c, so, blkEngine.bengine)
	var wengine world.Engine = engine
	if c.conf.GetVerbose() {
		wengine = world_vlogger.NewEngine(le, wengine)
	}

	// Publish the engine for the lifetime of this World.
	le.WithField("world-seqno", seqno).Info("world engine ready")
	c.engineCtr.SetValue(&wengine)
	defer c.engineCtr.SetValue(nil)

	// Follow accepted state into the World.
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
