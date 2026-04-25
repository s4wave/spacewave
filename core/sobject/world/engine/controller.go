package sobject_world_engine

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
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

// Config configures a World Graph engine bound to a block graph and controlled by a Shared Object.
// Uses MountSharedObject to mount and access the shared object and block store.
// Stores the HEAD reference in the Shared Object.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// engineCtr contains the engine object
	engineCtr *ccontainer.CContainer[*Engine]
	// engineID is the engine id we are listening on
	engineID string

	// processOpsAsValidator is the routine to process incoming operations as a validator.
	processOpsAsValidator *routine.RoutineContainer
	// gcSweepMaintenance is the routine to periodically queue GC sweep txs.
	gcSweepMaintenance *routine.RoutineContainer

	// sfs is the step factory set
	sfs *block_transform.StepFactorySet

	// writeBcast is broadcast after local commits and authoritative state updates.
	// Used by the GC sweep maintenance routine to detect world-state changes that
	// may leave pending GC journal entries.
	writeBcast broadcast.Broadcast

	// writeMtx guards write transactions / updating local state due to watching SOState.
	// only one of the two activities will be active at a time.
	writeMtx csync.Mutex

	// lastCommitResult caches the latest foreground commit for replay adoption.
	// Written during foreground writes (under writeMtx), read by watch-state
	// (under writeMtx) and validator (without writeMtx).
	lastCommitResult atomic.Pointer[commitResult]
}

// commitResult caches a foreground commit result for replay adoption.
// Replay consumers can adopt this result when the base root ref and
// op bytes match, avoiding expensive re-execution of processOp.
type commitResult struct {
	baseRootRef *block.BlockRef
	opData      []byte
	resultState *InnerState
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
		gcSweepMaintenance: routine.NewRoutineContainer(
			routine.WithExitLogger(le),
			routine.WithRetry(processBackoff),
		),

		sfs: sfs,
	}, nil
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
	le := c.le

	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Mount the shared object.
	le.Debug("mounting shared object")
	so, soRef, err := sobject.ExMountSharedObject(ctx, c.bus, c.conf.GetRef(), false, rctxCancel)
	if err != nil {
		return err
	}
	defer soRef.Release()

	// access the shared object state
	soStateCtr, soStateCtrRel, err := so.AccessSharedObjectState(ctx, rctxCancel)
	if err != nil {
		return err
	}
	defer soStateCtrRel()

	// start the process ops routine (as a validator)
	_, _ = c.processOpsAsValidator.SetRoutine(func(ctx context.Context) error {
		return c.executeProcessOpsAsValidator(ctx, so)
	})
	_ = c.processOpsAsValidator.SetContext(ctx, true)
	defer c.processOpsAsValidator.ClearContext()

	// init the shared object if necessary
	headState, err := c.loadOrInitHeadFromSharedObject(ctx, so, soStateCtr)
	if err != nil {
		return err
	}

	// last check if nil
	if headState.HeadRef == nil {
		headState.HeadRef = &bucket.ObjectRef{}
	}

	// the bucket ID is equivalent to the block store id
	bucketID := so.GetBlockStore().GetID()
	headState.HeadRef.BucketId = bucketID

	// verify transform config is not empty
	transformConf := headState.HeadRef.GetTransformConf()
	if len(transformConf.GetSteps()) == 0 {
		return sobject.ErrEmptyTransformConfig
	}

	// Build world state with engine
	blkEngine, err := c.buildBlkEngine(ctx, le, so, headState.HeadRef, transformConf)
	if err != nil {
		return err
	}
	defer blkEngine.Release()

	verbose := c.conf.GetVerbose()
	if verbose {
		le.
			WithField("world-root", headState.HeadRef.MarshalB58()).
			Debug("initialized world root")
	}

	// get initial seqno
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

	// Engine ready
	le.WithField("world-seqno", seqno).Info("world engine ready")
	c.engineCtr.SetValue(&wengine)
	defer c.engineCtr.SetValue(nil)

	// start the gc sweep maintenance routine (gated on validator/owner role)
	_, _ = c.gcSweepMaintenance.SetRoutine(func(ctx context.Context) error {
		return c.executeGCSweepMaintenance(ctx, so, blkEngine.bengine)
	})
	_ = c.gcSweepMaintenance.SetContext(ctx, true)
	defer c.gcSweepMaintenance.ClearContext()

	// Watch the SOState for changes.
	return c.executeWatchSOState(rctx, le, so, soStateCtr, engine)
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

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ world.Controller = ((*Controller)(nil))
