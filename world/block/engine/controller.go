package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	"github.com/golang/protobuf/proto"
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
}

// NewController constructs a new World Engine controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	return &Controller{
		le:       le,
		conf:     conf,
		bus:      bus,
		engineCh: make(chan EngineHandle, 1),
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"block world engine controller",
	)
}

// Execute executes the engine controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	le := c.le
	le.Debug("building world engine")

	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Determine the init ref to the HEAD
	var headRef *bucket.ObjectRef

	// initialize headRef using the configured head ref
	initRef := c.conf.GetInitHeadRef()
	if initRef != nil {
		var ok bool
		headRef, ok = proto.Clone(initRef).(*bucket.ObjectRef)
		if !ok {
			return block.ErrUnexpectedType
		}
	}

	// Lookup the state store
	stateStoreID := c.conf.GetObjectStoreId()
	stateStoreVol := c.conf.GetVolumeId()
	var stateStore object.ObjectStore
	if stateStoreID != "" && stateStoreVol != "" {
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
		if headRef.GetEmpty() {
			return errors.New("init head ref required if state store id or volume id are unset")
		}
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{}
	}
	if headState == nil {
		headState = &HeadState{}
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

	engine, err := world_block.NewEngine(ctx, cursor)
	if err != nil {
		return err
	}
	le.Info("world engine ready")

	c.engineCh <- world.NewEngineHandle(ctx, engine, nil)
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
var _ controller.Controller = ((*Controller)(nil))
