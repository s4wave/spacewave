package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_vlogger "github.com/aperturerobotics/hydra/world/vlogger"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Controller implements the block-graph World Engine controller.
// Attaches to a bucket to store blocks and a object store for state.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// engineCh contains the engine object
	engineCh chan EngineHandle
	// engineID is the engine id we are listening on
	engineID string

	// stateXfrm is the state transformer
	stateXfrm *block_transform.Transformer
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	sfs *block_transform.StepFactorySet,
) (*Controller, error) {
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		conf.GetStateTransformConf(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &Controller{
		le:       le.WithField("engine-id", conf.GetEngineId()),
		conf:     conf,
		bus:      bus,
		engineCh: make(chan EngineHandle, 1),
		engineID: conf.GetEngineId(),

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
	le := c.le

	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Determine the init ref to the HEAD
	var headRef *bucket.ObjectRef

	// initialize headRef using the configured head ref
	initRef := c.conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Lookup the state store
	stateStoreID := c.conf.GetObjectStoreId()
	stateStoreVol := c.conf.GetVolumeId()
	if stateStoreVol == "" {
		le.Debug("no volume id set, using any available volume")
	}

	var stateStore object.ObjectStore
	if stateStoreID != "" {
		storeVal, storeRef, err := volume.BuildObjectStoreAPIEx(ctx, c.bus, stateStoreID, stateStoreVol)
		if err != nil {
			return err
		}
		defer storeRef.Release()
		if err := storeVal.GetError(); err != nil {
			return err
		}
		stateStore = storeVal.GetObjectStore()
	}
	var headState *HeadState
	if stateStore != nil {
		// apply object store prefix
		if prefix := c.conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}
		// load initial head ref
		var headStateFound bool
		var err error
		headState, headStateFound, err = c.loadHeadState(ctx, stateStore)
		if err != nil {
			return err
		}
		if headStateFound {
			headRef = headState.GetHeadRef()
		}
	} else {
		le.Debug("state store is not configured, changes will not be persisted")
		if headRef.GetEmpty() {
			le.Debug("no initial head reference provided, initializing empty world")
		}
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{}
	}
	// override bucket id if configured
	if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return errors.New("head ref bucket id required but was unset")
	}

	// Build the initial cursor (will lookup the bucket)
	sfs, err := transform_all.BuildFactorySet() // TODO: commonize
	if err != nil {
		return err
	}

	le.Debug("building world engine")
	cursor, err := bucket_lookup.BuildCursor(
		ctx,
		c.bus,
		le,
		sfs,
		c.conf.GetVolumeId(),
		headRef,
		nil,
	)
	if err != nil {
		return err
	}
	defer cursor.Release()

	var lookupWorldOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		lookupWorldOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}

	var commitFn world_block.CommitFn
	if stateStore != nil {
		commitFn = func(nref *bucket.ObjectRef) error {
			// write state back to state store
			return c.writeHeadState(ctx, stateStore, nref)
		}
	}

	engine, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		lookupWorldOp,
		commitFn,
	)
	if err != nil {
		return err
	}

	le.Info("world engine ready")
	engine.SetVerbose(c.conf.GetVerbose())
	var wengine world.Engine = engine
	if c.conf.GetVerbose() {
		wengine = world_vlogger.NewEngine(le, wengine)
	}
	c.engineCh <- world.NewEngineHandle(ctx, wengine, nil)

	<-rctx.Done()
	le.Debug("shutting down")
	handle := <-c.engineCh
	handle.Release()

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	dir := di.GetDirective()
	// LookupWorldEngine handler.
	if d, ok := dir.(world.LookupWorldEngine); ok {
		return c.resolveLookupWorldEngine(ctx, di, d)
	}

	return nil, nil
}

// GetWorldEngine waits for the engine to be built.
// Returns a new EngineHandle, be sure to call Release when done.
func (c *Controller) GetWorldEngine(ctx context.Context) (EngineHandle, error) {
	select {
	case <-ctx.Done():
		return nil, context.Canceled
	case eng := <-c.engineCh:
		c.engineCh <- eng
		return world.NewEngineHandle(eng.GetContext(), eng, nil), nil
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ world.Controller = ((*Controller)(nil))
