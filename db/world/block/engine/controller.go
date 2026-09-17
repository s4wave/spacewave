package world_block_engine

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// Controller implements the block-graph World Engine controller.
// Attaches to a bucket to store blocks and a object store for state.
type Controller struct {
	// le is the controller logger.
	le *logrus.Entry
	// bus resolves storage and World operation controllers.
	bus bus.Bus
	// conf supplies immutable controller configuration.
	conf *Config
	// engineCtr contains the engine value or fatal startup error.
	engineCtr *ccontainer.CContainer[*engineResult]
	// engineID identifies the World engine exposed by directives.
	engineID string
	// mtx guards controller-lifetime executions and engine resources.
	mtx sync.Mutex
	// closed indicates controller resource cleanup has started.
	closed bool
	// closeDone closes after controller resource cleanup completes.
	closeDone chan struct{}
	// closeErr is the result returned to every Close caller.
	closeErr error
	// executions are canceled and joined by Close.
	executions map[*engineExecution]struct{}
	// engineResources remain valid across Execute restarts until Close.
	engineResources []engineResource

	// sfs resolves block transformation steps.
	sfs *block_transform.StepFactorySet
	// stateXfrm transforms persisted World-head metadata.
	stateXfrm *block_transform.Transformer
}

// engineResult publishes either a usable engine or a fatal initialization error.
type engineResult struct {
	engine Engine
	err    error
}

// engineExecution retains the cancellation and completion of one Execute call.
type engineExecution struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// engineResource keeps a World engine and its head store alive until Close.
type engineResource struct {
	engine   *world_block.Engine
	storeRef directive.Reference
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	sfs *block_transform.StepFactorySet,
) (*Controller, error) {
	// Validate the configured state transformation before retaining resources.
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		conf.GetStateTransformConf(),
	)
	if err != nil {
		return nil, err
	}

	// Retain execution and storage resources until controller Close.
	return &Controller{
		le:         le.WithField("engine-id", conf.GetEngineId()),
		conf:       conf,
		bus:        bus,
		engineCtr:  ccontainer.NewCContainer[*engineResult](nil),
		engineID:   conf.GetEngineId(),
		closeDone:  make(chan struct{}),
		executions: make(map[*engineExecution]struct{}),

		sfs:       sfs,
		stateXfrm: xfrm,
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
	// Register a cancelable execution before resolving storage.
	le := c.le
	rctx, rctxCancel := context.WithCancel(ctx)
	execution := &engineExecution{
		cancel: rctxCancel,
		done:   make(chan struct{}),
	}
	if !c.startExecution(execution) {
		rctxCancel()
		return nil
	}
	defer func() {
		rctxCancel()
		c.finishExecution(execution)
	}()
	ctx = rctx

	// Use the configured root until durable metadata supplies a newer head.
	var headRef *bucket.ObjectRef
	initRef := c.conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Resolve the configured state store within its Volume.
	stateStoreID := c.conf.GetObjectStoreId()
	stateStoreVol := c.conf.GetVolumeId()
	if stateStoreVol == "" {
		le.Debug("no volume id set, using any available volume")
	}

	// Release storage on initialization failure until retainEngine accepts it.
	var stateStoreRef directive.Reference
	defer func() {
		if stateStoreRef != nil {
			stateStoreRef.Release()
		}
	}()

	// Resolve the persisted head store and its coordination scope.
	var stateStore object.ObjectStore
	var stateCoordinator coord.Coordinator
	var stateCoordScope coord.Scope
	if stateStoreID != "" {
		storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, c.bus, false, stateStoreID, stateStoreVol, nil)
		if err != nil {
			return err
		}
		stateStoreRef = storeRef
		stateStore = storeVal.GetObjectStore()
		stateVolume := storeVal.GetVolume()
		stateCoordScope = coord.Scope{
			VolumeID:      storeVal.GetVolumeId(),
			ObjectStoreID: storeVal.GetID(),
			ParticipantID: c.engineID,
		}
		stateCoordinator = stateVolume
	}
	var persistedHeadRef *bucket.ObjectRef

	// Prefer the persisted head within the configured metadata prefix.
	var headState *HeadState
	if stateStore != nil {
		// Scope durable head metadata to the configured prefix.
		if prefix := c.conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}

		// Load the persisted head before constructing World state.
		var headStateFound bool
		var err error
		headState, headStateFound, err = c.loadHeadState(ctx, stateStore)
		if err != nil {
			return err
		}
		if headStateFound {
			headRef = headState.GetHeadRef()
			if headRef != nil {
				persistedHeadRef = headRef.Clone()
			}
		}
	} else {
		le.Debug("state store is not configured, changes will not be persisted")
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{}
	}

	// Bind the selected head to the configured bucket.
	if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return errors.New("head ref bucket id required but was unset")
	}

	// Permit one explicitly configured recovery from missing durable blocks.
	var recoveryBaseRef *bucket.ObjectRef
	var recoveredMissingPersistedHead bool
	recoverMissingPersistedHead := func(err error) bool {
		if err == nil ||
			!errors.Is(err, block.ErrNotFound) ||
			persistedHeadRef == nil ||
			recoveredMissingPersistedHead ||
			!c.conf.GetRecoverMissingPersistedHead() {
			return false
		}

		// Select the configured replacement while retaining the CAS base.
		recoveredMissingPersistedHead = true
		recoveryBaseRef = persistedHeadRef
		if initRef == nil {
			headRef = &bucket.ObjectRef{}
		} else {
			headRef = initRef.Clone()
		}
		if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
			headRef.BucketId = confBucketID
		}
		le.WithError(err).Warn("persisted world head is missing blocks; rebuilding world")
		return true
	}

buildWorldEngine:

	// Resolve and validate the selected root before publishing an engine.
	le.Debug("building world engine")
	cursor, err := bucket_lookup.BuildCursor(
		ctx,
		c.bus,
		le,
		c.sfs,
		c.conf.GetVolumeId(),
		headRef,
		nil,
	)
	if isReadOnlyInitHeadNotFound(err, stateStore, initRef) {
		c.engineCtr.SetValue(&engineResult{err: err})
		le.WithError(err).Warn("read-only world engine init head is missing")
		return nil
	}
	if err != nil {
		if recoverMissingPersistedHead(err) {
			goto buildWorldEngine
		}
		return err
	}
	if err := validateReadOnlyInitHead(ctx, cursor, stateStore, initRef); err != nil {
		c.engineCtr.SetValue(&engineResult{err: err})
		le.WithError(err).Warn("read-only world engine init head is missing")
		cursor.Release()
		return nil
	}

	// Persist a new empty World before enabling coordinated readers.
	if headRef.GetRootRef().GetEmpty() {
		le.Debug("no initial head reference provided, building new world")
		btx, bcs := cursor.BuildTransaction(nil)
		worldRoot := world_block.NewWorld(c.conf.GetDisableChangelog())
		bcs.ClearAllRefs()
		bcs.SetBlock(worldRoot, true)
		nrootRef, _, err := btx.Write(ctx, true)
		if err != nil {
			cursor.Release()
			return err
		}
		headRef.RootRef = nrootRef
		cursor.SetRootRef(nrootRef)
		if stateStore != nil {
			if err := c.writeHeadState(ctx, stateStore, recoveryBaseRef, headRef.Clone()); err != nil {
				cursor.Release()
				return err
			}
			recoveryBaseRef = nil
		}
	}

	// Resolve World operations through the configured controller bus.
	var lookupWorldOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		lookupWorldOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}

	// Report the selected root when verbose diagnostics are enabled.
	verbose := c.conf.GetVerbose()
	if verbose {
		le.
			WithField("world-root", headRef.MarshalB58()).
			Debug("initialized root")
	}

	// Persist each accepted head through the ObjectStore CAS contract.
	var commitFn world_block.CommitFn = func(ctx context.Context, baseRef, nref *bucket.ObjectRef) error {
		if verbose {
			le.
				WithField("world-root", nref.MarshalB58()).
				Debug("updated root")
		}
		if stateStore != nil {
			return c.writeHeadState(ctx, stateStore, baseRef, nref)
		}
		return nil
	}

	// Require durable generations before enabling coordinated transactions.
	useStateCoordinator := false
	if stateCoordinator != nil && stateStore != nil {
		useStateCoordinator = c.coordinatorSupported(ctx, stateCoordinator, stateCoordScope)
	}

	// Enable single-writer deferred durability: in non-coordinator mode block
	// writes and the durable head batch until Sync (the clean-shutdown flush
	// below and explicit consumer fences). Ignored when the write coordinator is
	// active, which keeps durable-on-write per-commit CAS semantics.
	engineOpts := []world_block.EngineOption{world_block.WithDeferredDurability()}
	if useStateCoordinator {
		engineOpts = append(engineOpts, world_block.WithWriteCoordinator(
			stateCoordinator,
			stateCoordScope,
			c.objectStoreHeadKeyPrefix(),
			c.refreshDurableHeadRef(stateStore),
		))
	}

	// Only fuse blocks and metadata when both resolve to the same atomic volume.
	// Otherwise retain the existing Sync + validated ObjectStore CAS path.
	if publisher, ok := cursor.GetBucket().(block.AtomicPublisher); ok &&
		useStateCoordinator && publisher.SupportsAtomicPublication() &&
		publisher.AtomicPublicationVolumeID() == stateCoordScope.VolumeID {
		engineOpts = append(engineOpts, world_block.WithAtomicPublication(publisher,
			func(baseRef, nref *bucket.ObjectRef) *block.AtomicHeadUpdate {
				baseRef, nref = baseRef.Clone(), nref.Clone()
				return &block.AtomicHeadUpdate{
					ObjectStoreID: stateCoordScope.ObjectStoreID,
					Key:           c.objectStoreHeadKeyPrefix(),
					Replace: func(ctx context.Context, data []byte, found bool) ([]byte, error) {
						return c.replaceHeadState(ctx, data, found, baseRef, nref)
					},
				}
			}))
	}

	// Construct the engine with its storage and observation policies.
	engine, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		lookupWorldOp,
		commitFn,
		c.conf.GetVerbose(),
		engineOpts...,
	)
	if err != nil {
		if recoverMissingPersistedHead(err) {
			goto buildWorldEngine
		}
		return err
	}

	// NewEngine validates the replacement root before recovery publishes it.
	// Publish before GetSeqno can refresh the coordinator's persisted head.
	if recoveryBaseRef != nil {
		if err := c.writeHeadState(ctx, stateStore, recoveryBaseRef, headRef.Clone()); err != nil {
			_ = engine.Close()
			return err
		}
		recoveryBaseRef = nil
	}

	// Verify the published head is readable before exposing the engine.
	seqno, err := engine.GetSeqno(ctx)
	if isReadOnlyInitHeadNotFound(err, stateStore, initRef) {
		c.engineCtr.SetValue(&engineResult{err: err})
		le.WithError(err).Warn("read-only world engine init head is missing")
		_ = engine.Close()
		return nil
	}
	if err != nil {
		_ = engine.Close()
		if recoverMissingPersistedHead(err) {
			goto buildWorldEngine
		}
		return err
	}

	// Retain the engine and its store reference across execution restarts.
	le.WithField("world-seqno", seqno).Info("world engine ready")
	var wengine world.Engine = engine
	if c.conf.GetVerbose() {
		wengine = world_vlogger.NewEngine(le, wengine)
	}
	if !c.retainEngine(engine, wengine, stateStoreRef) {
		_ = engine.Close()
		return nil
	}
	stateStoreRef = nil

	// Fence durability when this execution ends.
	<-rctx.Done()
	le.Debug("shutting down")
	// Clean-shutdown durability flush. In the single-writer path the durable
	// head advances only at Sync, so a normal shutdown must fence before ending
	// this execution. Controller-lifetime resources remain mounted until Close.
	if _, err := engine.Sync(context.WithoutCancel(ctx)); err != nil {
		le.WithError(err).Warn("world engine shutdown sync failed")
	}
	c.engineCtr.SetValue(nil)
	rctxCancel()
	return nil
}

// coordinatorSupported requires durable generation fencing for direct writers.
func (c *Controller) coordinatorSupported(
	ctx context.Context,
	coordinator coord.Coordinator,
	scope coord.Scope,
) bool {
	capability, err := coordinator.Capability(ctx, scope)
	if err != nil {
		if ctx.Err() == nil {
			c.le.WithError(err).Warn("world coordinator capability lookup failed")
		}
		return false
	}
	return capability != nil && capability.Supported && capability.Generations
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Resolve only World engine lookups.
	dir := di.GetDirective()
	if d, ok := dir.(world.LookupWorldEngine); ok {
		return directive.R(c.resolveLookupWorldEngine(ctx, di, d))
	}

	// Leave unrelated directives unresolved.
	return nil, nil
}

// GetWorldEngine waits for the engine to be built.
// Returns the Engine managed by the controller.
func (c *Controller) GetWorldEngine(ctx context.Context) (Engine, error) {
	result, err := c.engineCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	if result.err != nil {
		return nil, result.err
	}
	return result.engine, nil
}

// validateReadOnlyInitHead rejects a configured immutable root absent from its bucket.
func validateReadOnlyInitHead(
	ctx context.Context,
	cursor *bucket_lookup.Cursor,
	stateStore object.ObjectStore,
	initRef *bucket.ObjectRef,
) error {
	if !isReadOnlyInitHead(stateStore, initRef) {
		return nil
	}
	found, err := cursor.GetBucket().GetBlockExists(ctx, initRef.GetRootRef())
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	return errors.Wrap(block.ErrNotFound, initRef.GetRootRef().MarshalString())
}

// isReadOnlyInitHead identifies a configured root without a writable head store.
func isReadOnlyInitHead(stateStore object.ObjectStore, initRef *bucket.ObjectRef) bool {
	return stateStore == nil &&
		initRef != nil &&
		!initRef.GetRootRef().GetEmpty()
}

// isReadOnlyInitHeadNotFound identifies fatal missing immutable startup roots.
func isReadOnlyInitHeadNotFound(err error, stateStore object.ObjectStore, initRef *bucket.ObjectRef) bool {
	return err != nil &&
		errors.Is(err, block.ErrNotFound) &&
		isReadOnlyInitHead(stateStore, initRef)
}

// startExecution retains a serving execution unless controller cleanup has begun.
func (c *Controller) startExecution(execution *engineExecution) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.closed {
		return false
	}
	c.executions[execution] = struct{}{}
	return true
}

// finishExecution removes a completed execution and releases Close waiters.
func (c *Controller) finishExecution(execution *engineExecution) {
	c.mtx.Lock()
	delete(c.executions, execution)
	close(execution.done)
	c.mtx.Unlock()
}

// retainEngine publishes an initialized engine and retains its store until Close.
func (c *Controller) retainEngine(
	engine *world_block.Engine,
	publishedEngine world.Engine,
	storeRef directive.Reference,
) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.closed {
		return false
	}
	c.engineResources = append(c.engineResources, engineResource{
		engine:   engine,
		storeRef: storeRef,
	})
	c.engineCtr.SetValue(&engineResult{engine: publishedEngine})
	return true
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// Detach resources once and join any cleanup already in progress.
	c.mtx.Lock()
	if c.closed {
		closeDone := c.closeDone
		c.mtx.Unlock()
		<-closeDone
		c.mtx.Lock()
		closeErr := c.closeErr
		c.mtx.Unlock()
		return closeErr
	}
	c.closed = true
	executions := make([]*engineExecution, 0, len(c.executions))
	for execution := range c.executions {
		executions = append(executions, execution)
	}
	resources := c.engineResources
	c.engineResources = nil
	c.engineCtr.SetValue(nil)
	c.mtx.Unlock()

	// Stop and join executions before releasing their retained resources.
	for _, execution := range executions {
		execution.cancel()
	}
	for _, execution := range executions {
		<-execution.done
	}

	// Close engines and their watchers before releasing state stores.
	var closeErr error
	for _, resource := range resources {
		if err := resource.engine.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		if resource.storeRef != nil {
			resource.storeRef.Release()
		}
	}

	// Publish one cleanup result to concurrent Close callers.
	c.mtx.Lock()
	c.closeErr = closeErr
	close(c.closeDone)
	c.mtx.Unlock()
	return closeErr
}

// _ verifies the World controller contract.
var _ world.Controller = (*Controller)(nil)
