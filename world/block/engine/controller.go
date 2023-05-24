package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_vlogger "github.com/aperturerobotics/hydra/world/vlogger"
	"github.com/aperturerobotics/util/ccontainer"
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
	// engineCtr contains the engine object
	engineCtr *ccontainer.CContainer[*Engine]
	// engineID is the engine id we are listening on
	engineID string

	// sfs is the step factory set
	sfs *block_transform.StepFactorySet
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
	)
	if err != nil {
		return nil, err
	}

	return &Controller{
		le:        le.WithField("engine-id", conf.GetEngineId()),
		conf:      conf,
		bus:       bus,
		engineCtr: ccontainer.NewCContainer[*Engine](nil),
		engineID:  conf.GetEngineId(),

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
		storeVal, _, storeRef, err := volume.BuildObjectStoreAPIEx(ctx, c.bus, false, stateStoreID, stateStoreVol, nil)
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
	if err != nil {
		return err
	}
	defer cursor.Release()

	if headRef.GetRootRef().GetEmpty() {
		le.Debug("no initial head reference provided, building new world")
		btx, bcs := cursor.BuildTransaction(nil)
		worldRoot := world_block.NewWorld(c.conf.GetDisableChangelog())
		bcs.ClearAllRefs()
		bcs.SetBlock(worldRoot, true)
		nrootRef, _, err := btx.Write(true)
		if err != nil {
			return err
		}
		headRef.RootRef = nrootRef
		cursor.SetRootRef(nrootRef)
	}

	var lookupWorldOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		lookupWorldOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}

	verbose := c.conf.GetVerbose()
	if verbose {
		le.Debugf("init root world-block-engine(%s)", headRef.MarshalB58())
	}

	var commitFn world_block.CommitFn = func(nref *bucket.ObjectRef) error {
		if verbose {
			le.
				WithField("world-root", nref.MarshalB58()).
				Debug("updated root")
		}
		if stateStore != nil {
			// write state back to state store
			return c.writeHeadState(ctx, stateStore, nref)
		}
		return nil
	}

	engine, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		lookupWorldOp,
		commitFn,
		c.conf.GetVerbose(),
	)
	if err != nil {
		return err
	}

	le.Info("world engine ready")
	var wengine world.Engine = engine
	if c.conf.GetVerbose() {
		wengine = world_vlogger.NewEngine(le, wengine)
	}
	c.engineCtr.SetValue(&wengine)

	<-rctx.Done()
	le.Debug("shutting down")
	c.engineCtr.SetValue(nil)

	return nil
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
