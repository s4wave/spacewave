package dist_entrypoint

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/refcount"
	"github.com/blang/semver/v4"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_kvfile "github.com/s4wave/spacewave/db/block/store/kvfile"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/sirupsen/logrus"
)

// StaticBlockStore manages the static kvfile block store.
type StaticBlockStore struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// blockStoreID is the block store id to use
	blockStoreID string
	// kvkey controls the keys in the kvfile
	kvkey *store_kvkey.KVKey
	// bucketIDs is the set of bucket ids to handle w/ the block store
	bucketIDs []string
	// buildReader builds the reader
	buildReader refcount.RefCountResolver[*kvfile.Reader]
	// close is the close callback
	close func()
}

// NewStaticBlockStore constructs a new static block store controller.
func NewStaticBlockStore(
	le *logrus.Entry,
	b bus.Bus,
	blockStoreID string,
	buildReader refcount.RefCountResolver[*kvfile.Reader],
	kvkey *store_kvkey.KVKey,
	bucketIDs []string,
	close func(),
) *StaticBlockStore {
	return &StaticBlockStore{
		le:           le,
		b:            b,
		blockStoreID: blockStoreID,
		buildReader:  buildReader,
		kvkey:        kvkey,
		bucketIDs:    bucketIDs,
		close:        close,
	}
}

// GetControllerInfo returns information about the controller.
func (c *StaticBlockStore) GetControllerInfo() *controller.Info {
	return controller.NewInfo("entrypoint/static", semver.MustParse("0.0.1"), "entrypoint static block store loader")
}

// HandleDirective asks if the handler can resolve the directive.
func (c *StaticBlockStore) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Execute executes the controller goroutine.
func (c *StaticBlockStore) Execute(ctx context.Context) error {
	ctrl := block_store_controller.NewController(
		c.le,
		controller.NewInfo(
			"entrypoint/static/block-store",
			semver.MustParse("0.0.1"),
			"entrypoint static block store",
		),
		func(ctx context.Context, released func()) (block_store.Store, func(), error) {
			reader, readerRel, err := c.buildReader(ctx, released)
			if err != nil {
				return nil, readerRel, err
			}

			storeOps := block_store_kvfile.NewKvfileBlock(ctx, c.kvkey, reader)
			store := block_store.NewStore(bldr_dist.StaticBlockStoreID, storeOps)
			return store, readerRel, nil
		},
		[]string{bldr_dist.StaticBlockStoreID},
		false,
		c.bucketIDs,
		true,
		false,
	)

	relCtrl, err := c.b.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}

	context.AfterFunc(ctx, relCtrl)
	return nil
}

// Close releases any resources used by the controller.
func (c *StaticBlockStore) Close() error {
	if c.close != nil {
		c.close()
	}
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*StaticBlockStore)(nil))
