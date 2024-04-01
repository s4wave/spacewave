package block_store_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/refcount"
	"github.com/sirupsen/logrus"
)

// Controller resolves LookupBlockStore with a block_store.Store.
type Controller struct {
	// info is the controller info
	info *controller.Info
	// storeCtr contains the block store
	storeCtr *ccontainer.CContainer[block_store.Store]
	// errCtr contains any error building the handler
	errCtr *ccontainer.CContainer[*error]
	// rc is the refcount container
	rc *refcount.RefCount[block_store.Store]
	// blockStoreIds is the list of block store ids to match
	// ignores if empty
	blockStoreIds []string
	// bucketIDs is a list of bucket ids to resolve LookupBlockFromNetwork directives.
	bucketIDs []string
	// skipNotFound returns no value if not found. otherwise returns found=false
	skipNotFound bool
	// verbose wraps the block store with a verbose logger
	verbose bool
}

// NewController constructs a new controller.
//
// blockStoreIds is the list of block store ids to match LookupBlockStore.
// buildOnStart adds a reference on startup & always runs the block store.
// bucketIDs is a list of bucket ids to resolve LookupBlockFromNetwork directives.
func NewController(
	le *logrus.Entry,
	info *controller.Info,
	resolver BlockStoreBuilder,
	blockStoreIds []string,
	buildOnStart bool,
	bucketIDs []string,
	skipNotFound,
	verbose bool,
) *Controller {
	h := &Controller{
		info:          info,
		storeCtr:      ccontainer.NewCContainer[block_store.Store](nil),
		errCtr:        ccontainer.NewCContainer[*error](nil),
		blockStoreIds: blockStoreIds,
		bucketIDs:     bucketIDs,
		skipNotFound:  skipNotFound,
		verbose:       verbose,
	}
	if verbose && resolver != nil {
		resolver = WrapVerboseBlockStoreBuilder(le, resolver)
	}
	keepUnref := buildOnStart // never unreferenced if true, but set anyway.
	h.rc = refcount.NewRefCount(nil, keepUnref, h.storeCtr, h.errCtr, resolver)
	if buildOnStart {
		_ = h.rc.AddRef(nil)
	}
	return h
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// WaitBlockStore adds a reference to the block store and waits for it to be constructed.
func (c *Controller) WaitBlockStore(ctx context.Context) (block_store.Store, *refcount.Ref[block_store.Store], error) {
	storePromise, storeRef := c.AddBlockStoreRef()
	store, err := storePromise.Await(ctx)
	if err != nil {
		storeRef.Release()
		return nil, nil, err
	}
	return store, storeRef, nil
}

// AddBlockStoreRef adds a reference to the block store.
func (c *Controller) AddBlockStoreRef() (promise.PromiseLike[block_store.Store], *refcount.Ref[block_store.Store]) {
	return c.rc.AddRefPromise()
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	c.rc.SetContext(ctx)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case block_store.LookupBlockStore:
		storeID := d.LookupBlockStoreId()
		var matched bool
		for _, id := range c.blockStoreIds {
			if id == storeID {
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil
		}
		return directive.R(directive.NewRefCountResolver(c.rc, true, func(ctx context.Context, store block_store.Store) (directive.Value, error) {
			if store == nil {
				return nil, nil
			}
			return block_store.LookupBlockStoreValue(store), nil
		}), nil)
	case dex.LookupBlockFromNetwork:
		return c.resolveLookupBlockFromNetwork(ctx, inst, d)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	c.rc.ClearContext()
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
