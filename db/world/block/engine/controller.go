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
	// decodedBlocks shares decoded blocks across every engine transaction until Close.
	decodedBlocks *block.DecodedBlockCache

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

		decodedBlocks: block.NewDecodedBlockCache(),

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

	// Release storage on initialization failure until retainEngine accepts it.
	var stateStoreRef directive.Reference
	defer func() {
		if stateStoreRef != nil {
			stateStoreRef.Release()
		}
	}()

	// Resolve the object store an older engine kept the head in, and the
	// coordinator of its Volume.
	var stateStore object.ObjectStore
	var stateStoreID string
	var coordinator coord.Coordinator
	scopeVolumeID := c.conf.GetVolumeId()
	if id := c.conf.GetObjectStoreId(); id != "" {
		storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, c.bus, false, id, c.conf.GetVolumeId(), nil)
		if err != nil {
			return err
		}
		stateStoreRef = storeRef
		stateStore = storeVal.GetObjectStore()
		if prefix := c.conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}
		stateStoreID = storeVal.GetID()
		coordinator = storeVal.GetVolume()
		scopeVolumeID = storeVal.GetVolumeId()
	}

	// Bind the configured bucket. A World with no object store and a
	// configured root is read-only.
	initRef := c.conf.GetInitHeadRef()
	readOnly := isReadOnlyInitHead(stateStore, initRef)
	headRef := &bucket.ObjectRef{}
	if initRef != nil {
		headRef = initRef.Clone()
	}
	if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return errors.New("head ref bucket id required but was unset")
	}

	// Resolve the bucket's cursor.
	le.Debug("building world engine")
	cursor, err := bucket_lookup.BuildCursor(ctx, c.bus, le, c.sfs, c.conf.GetVolumeId(), headRef, nil)
	if isReadOnlyInitHeadNotFound(err, stateStore, initRef) {
		c.engineCtr.SetValue(&engineResult{err: err})
		le.WithError(err).Warn("read-only world engine init head is missing")
		return nil
	}
	if err != nil {
		return err
	}
	cursor.SetDecodedBlockCache(c.decodedBlocks)

	// Open a read-only World at its configured root, or a writable World at
	// the head its authority holds.
	var lookupWorldOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		lookupWorldOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}
	var engine *world_block.Engine
	if readOnly {
		engine, err = c.openReadOnly(ctx, cursor, initRef, lookupWorldOp)
	} else {
		engine, err = c.openWritable(ctx, cursor, stateStore, stateStoreID, coordinator, scopeVolumeID, lookupWorldOp)
	}
	if isReadOnlyInitHeadNotFound(err, stateStore, initRef) {
		c.engineCtr.SetValue(&engineResult{err: err})
		le.WithError(err).Warn("read-only world engine init head is missing")
		cursor.Release()
		return nil
	}
	if err != nil {
		cursor.Release()
		return err
	}

	// Apply the configured changelog setting to an existing World.
	if !readOnly {
		if err := engine.SetChangelogDisabled(ctx, c.conf.GetDisableChangelog()); err != nil {
			_ = engine.Close()
			return err
		}
	}

	// Retain the engine and its store reference across execution restarts.
	seqno, _ := engine.GetSeqno(ctx)
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

	// Move an existing World to the configured graph settings in the
	// background, alongside writers.
	var graphSettingsDone chan struct{}
	if target := c.conf.GetGraphSettings(); len(target.GetIndexes()) != 0 && !readOnly {
		graphSettingsDone = make(chan struct{})
		go func() {
			defer close(graphSettingsDone)
			driveGraphSettings(rctx, le, engine, target)
		}()
	}

	// Fence durability when this execution ends, after the settings change
	// stops. Controller-lifetime resources remain mounted until Close.
	<-rctx.Done()
	le.Debug("shutting down")
	if graphSettingsDone != nil {
		<-graphSettingsDone
	}
	if _, err := engine.Sync(context.WithoutCancel(ctx)); err != nil {
		le.WithError(err).Warn("world engine shutdown sync failed")
	}
	c.engineCtr.SetValue(nil)
	rctxCancel()
	return nil
}

// openReadOnly opens the World at its configured root with no authority: its
// commits stay in memory and are never persisted.
func (c *Controller) openReadOnly(
	ctx context.Context,
	cursor *bucket_lookup.Cursor,
	initRef *bucket.ObjectRef,
	lookupWorldOp world.LookupOp,
) (*world_block.Engine, error) {
	// Require the configured root to exist in its bucket.
	if err := validateReadOnlyInitHead(ctx, cursor, nil, initRef); err != nil {
		return nil, err
	}

	// Open the engine at the root.
	return world_block.NewEngine(ctx, c.le, cursor, lookupWorldOp, nil, c.conf.GetVerbose(), world_block.WithDeferredDurability())
}

// openWritable opens the World at the head its local authority holds under
// the world root, adopting or creating the head first when the root is free,
// and replacing a head whose blocks are missing when configured to.
func (c *Controller) openWritable(
	ctx context.Context,
	cursor *bucket_lookup.Cursor,
	stateStore object.ObjectStore,
	stateStoreID string,
	coordinator coord.Coordinator,
	scopeVolumeID string,
	lookupWorldOp world.LookupOp,
) (*world_block.Engine, error) {
	// A writable World names its head root after its engine.
	if c.engineID == "" {
		return nil, world.ErrEmptyEngineID
	}
	name := world_block.WorldRootName(c.engineID)

	// Order the writers of every process when the Volume's coordinator can.
	scope := coord.Scope{VolumeID: scopeVolumeID, ObjectStoreID: name, ParticipantID: c.engineID}
	if coordinator != nil {
		supported, err := c.coordinatorSupported(ctx, coordinator, scope)
		if err != nil {
			return nil, err
		}
		if !supported {
			coordinator = nil
		}
	}
	authority := world_block.NewLocalAuthority(c.le, cursor, name, coordinator, scope)

	// Read the head, adopting or creating it when the root is free.
	legacy := legacyHead{store: stateStore, key: c.objectStoreHeadKey(), rootName: retainedRootName(stateStoreID, c.objectStoreHeadKeyPrefix())}
	head, err := c.openHead(ctx, authority, cursor, legacy)
	if err != nil {
		return nil, err
	}

	// Open the engine at the head, replacing a head with missing blocks once
	// when configured to.
	recovered := false
	for {
		cursor.SetRootRef(head.Root)
		engine, err := world_block.NewEngine(ctx, c.le, cursor, lookupWorldOp, authority, c.conf.GetVerbose())
		if err == nil || recovered || !errors.Is(err, block.ErrNotFound) || !c.conf.GetRecoverMissingPersistedHead() {
			return engine, err
		}
		c.le.WithError(err).Warn("world head is missing blocks; rebuilding world")
		recovered = true
		root, err := c.initialRoot(ctx, cursor)
		if err != nil {
			return nil, err
		}
		if head, err = authority.Install(ctx, head.Root, root); err != nil {
			return nil, err
		}
	}
}

// coordinatorSupported reports whether the coordinator orders the World's
// writers with durable generations. A coordinator that reports the scope
// unsupported answers false; a failed lookup fails the open, which the loader
// retries, rather than opening a shared World without a lease or a watch.
func (c *Controller) coordinatorSupported(
	ctx context.Context,
	coordinator coord.Coordinator,
	scope coord.Scope,
) (bool, error) {
	capability, err := coordinator.Capability(ctx, scope)
	if errors.Is(err, coord.ErrUnsupported) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "world coordinator capability")
	}
	return capability != nil && capability.Supported && capability.Generations, nil
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
	// Wait for the controller to publish its engine or startup failure.
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
	// Limit bucket validation to immutable configured World roots.
	if !isReadOnlyInitHead(stateStore, initRef) {
		return nil
	}

	// Require the configured World root to exist in its bucket.
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
	// Hold the controller resource lock while checking execution eligibility.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.closed {
		return false
	}

	// Retain the execution so controller cleanup can cancel and join it.
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
	// Hold the controller resource lock while checking engine eligibility.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.closed {
		return false
	}

	// Retain the engine storage and publish the usable World engine.
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

	// Detach the controller executions and engines for exclusive cleanup.
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
	c.decodedBlocks.Close()

	// Publish one cleanup result to concurrent Close callers.
	c.mtx.Lock()
	c.closeErr = closeErr
	close(c.closeDone)
	c.mtx.Unlock()
	return closeErr
}

// _ verifies the World controller contract.
var _ world.Controller = (*Controller)(nil)
