package cdn_world_controller

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/sobject"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the CDN world controller.
const ControllerID = "spacewave/cdn/world"

// Version is the version of the world implementation.
var Version = controller.MustParseVersion("0.0.1")

const missingHeadRetryDelay = time.Second

// Controller exposes a read-only CDN-backed world engine.
type Controller struct {
	le     *logrus.Entry
	b      bus.Bus
	conf   *Config
	engine *cdn_sharedobject.WorldEngine
	ctr    *ccontainer.CContainer[world.Engine]
}

// NewController builds a new CDN world controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return &Controller{
		le:   le.WithField("engine-id", conf.GetEngineId()),
		b:    b,
		conf: conf,
		ctr:  ccontainer.NewCContainer[world.Engine](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "CDN world controller: "+c.conf.GetEngineId())
}

func (c *Controller) newBlockStore(ctx context.Context) (*cdn_bstore.CdnBlockStore, func(), error) {
	pointerTTL, _ := c.conf.ParsePointerTTLDur()
	cacheID := c.conf.GetCacheBlockStoreId()
	var indexCache packfile_store.IndexCache
	var cacheStore block_store.LookupBlockStoreValue
	var releaseCache, releaseIndex func()
	releaseRefs := func() {
		if releaseIndex != nil {
			releaseIndex()
		}
		if releaseCache != nil {
			releaseCache()
		}
	}
	if cacheID != "" {
		var err error
		var cacheRef directive.Reference
		cacheStore, _, cacheRef, err = block_store.ExLookupFirstBlockStore(ctx, c.b, cacheID, false, nil)
		if err != nil {
			return nil, nil, err
		}
		releaseCache = cacheRef.Release
		objHandle, _, objRef, err := volume.ExBuildObjectStoreAPI(
			ctx,
			c.b,
			true,
			cdn_bstore.PackIndexObjectStoreID(c.conf.GetSpaceId()),
			cacheID,
			nil,
		)
		if err != nil {
			releaseRefs()
			return nil, nil, err
		}
		if objHandle != nil {
			releaseIndex = objRef.Release
			indexCache = manifest.NewIndexCache(objHandle.GetObjectStore())
		}
	}
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: c.conf.GetCdnBaseUrl(),
		SpaceID:    c.conf.GetSpaceId(),
		HttpClient: http.DefaultClient,
		PointerTTL: pointerTTL,
		IndexCache: indexCache,
	})
	if err != nil {
		releaseRefs()
		return nil, nil, err
	}
	if cacheID != "" {
		store.SetWriteback(ctx, cacheStore, c.conf.GetWritebackWindowBytes())
	}
	return store, func() {
		store.Close()
		releaseRefs()
	}, nil
}

// Execute builds the CDN world engine and holds it until shutdown.
func (c *Controller) Execute(ctx context.Context) error {
	store, releaseStore, err := c.newBlockStore(ctx)
	if err != nil {
		return err
	}
	defer releaseStore()
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    c.conf.GetSpaceId(),
		BlockStore: store,
	})
	if err != nil {
		return err
	}
	for {
		engine, err := cdn_sharedobject.NewWorldEngine(ctx, c.le, c.b, so, space_world_optypes.LookupWorldOp)
		if err != nil {
			if !shouldRetryMissingPublishedHead() || !isMissingPublishedHead(err) {
				return err
			}
			c.le.WithError(err).Debug("CDN world engine waiting for published head")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(missingHeadRetryDelay):
				continue
			}
		}
		c.engine = engine
		c.ctr.SetValue(engine.Engine)
		c.le.Info("CDN world engine ready")
		<-ctx.Done()
		engine.Release()
		c.engine = nil
		c.ctr.SetValue(nil)
		return nil
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(world.LookupWorldEngine)
	if !ok {
		return nil, nil
	}
	if id := dir.LookupWorldEngineID(); id != "" && id != c.conf.GetEngineId() {
		return nil, nil
	}
	return directive.R(world.NewWorldEngineResolver(c))
}

// GetWorldEngine waits for the engine to be built.
func (c *Controller) GetWorldEngine(ctx context.Context) (world.Engine, error) {
	return c.ctr.WaitValue(ctx, nil)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	if c.engine != nil {
		c.engine.Release()
		c.engine = nil
	}
	return nil
}

func isMissingPublishedHead(err error) bool {
	health, ok := sobject.GetSharedObjectHealthFromError(err)
	if !ok || health == nil {
		return false
	}
	return health.GetStatus() == sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING &&
		health.GetLayer() == sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT
}

// _ is a type assertion.
var (
	_ controller.Controller = (*Controller)(nil)
	_ world.Controller      = (*Controller)(nil)
)
