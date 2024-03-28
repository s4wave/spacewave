package block_store_bucket

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/blang/semver"
)

// ControllerID is the controller id.
const ControllerID = "hydra/block/store/bucket"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "block-store backed bucket"

// Controller is the block store bucket controller.
type Controller struct {
	bucketStoreID    string
	bucketConf       *bucket.Config
	accessBlockStore block_store.AccessBlockStoreFunc
	errCtr           *ccontainer.CContainer[*error]
	handleRc         *refcount.RefCount[bucket.BucketHandle]
}

// NewController constructs the controller.
//
// If bucketStoreID is set, filters the bucketStoreID field on BuildBucketAPI.
func NewController(
	bucketStoreID string,
	bucketConf *bucket.Config,
	accessBlockStore block_store.AccessBlockStoreFunc,
) *Controller {
	ctrl := &Controller{
		bucketStoreID:    bucketStoreID,
		bucketConf:       bucketConf,
		accessBlockStore: accessBlockStore,
	}
	// note: keep the handle if we have zero references and the context is not canceled.
	ctrl.errCtr = ccontainer.NewCContainer[*error](nil)
	ctrl.handleRc = refcount.NewRefCount(nil, true, nil, ctrl.errCtr, ctrl.resolveBucketHandle)
	return ctrl
}

// GetBucketStoreId returns the bucket store id used for store_id in BuildBucketAPI.
func (c *Controller) GetBucketStoreId() string {
	return c.bucketStoreID
}

// GetBucketConf returns the bucket config.
func (c *Controller) GetBucketConf() *bucket.Config {
	return c.bucketConf
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		controllerDescrip,
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.handleRc.SetContext(ctx)
	defer c.handleRc.SetContext(nil)

	rerr, err := c.errCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	return *rerr
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bucket.BuildBucketAPI:
		return directive.R(c.ResolveBuildBucketAPI(ctx, di, d), nil)
	}
	return nil, nil
}

// ResolveBuildBucketAPI resolves an BuildBucketAPI directive if the bucket id matches.
func (c *Controller) ResolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	d bucket.BuildBucketAPI,
) directive.Resolver {
	bucketID := d.BuildBucketAPIBucketID()
	if bucketID != c.bucketConf.GetId() {
		return nil
	}

	storeID := d.BuildBucketAPIStoreID()
	if storeID != c.bucketStoreID {
		return nil
	}

	return directive.NewRefCountResolver(c.handleRc, false, nil)
}

// BuildBucketAPI accesses the bucket handle.
func (c *Controller) BuildBucketAPI(ctx context.Context, released func()) (bucket.BucketHandle, func(), error) {
	valProm, valRef := c.handleRc.WaitWithReleased(ctx, released)
	val, err := valProm.Await(ctx)
	if err != nil {
		valRef.Release()
		return nil, nil, err
	}
	return val, valRef.Release, nil
}

// resolveBucketHandle resolves building the bucket handle.
func (c *Controller) resolveBucketHandle(ctx context.Context, released func()) (bucket.BucketHandle, func(), error) {
	// Access the block store.
	blockStore, relBlockStore, err := c.accessBlockStore(ctx, released)
	if err == nil && blockStore == nil {
		err = block_store.ErrBlockStoreNotFound
	}
	if err != nil {
		if relBlockStore != nil {
			relBlockStore()
		}
		return nil, nil, err
	}

	// Wrap into the bucket handle
	bkt := NewBucket(blockStore, c.bucketConf)
	return bucket.NewBucketHandle(c.bucketConf.GetId(), c.bucketStoreID, bkt), relBlockStore, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	c.handleRc.ClearContext()
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = ((*Controller)(nil))
	_ bucket.BucketHandle   = (bucket.BuildBucketAPIValue)(nil)
)
