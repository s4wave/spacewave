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

const releaseWorldEngineID = "spacewave-release-world"

// ReleaseBlockStoreID identifies the block store shared by Release World
// metadata reads and release-manifest bucket reads.
const ReleaseBlockStoreID = "spacewave-release-cdn"

// Controller exposes a read-only CDN-backed world engine.
type Controller struct {
	le       *logrus.Entry
	b        bus.Bus
	conf     *Config
	engine   *cdn_sharedobject.WorldEngine
	ctr      *ccontainer.CContainer[world.Engine]
	storeCtr *ccontainer.CContainer[*blockStoreAuthority]
}

// NewController builds a new CDN world controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return &Controller{
		le:       le.WithField("engine-id", conf.GetEngineId()),
		b:        b,
		conf:     conf,
		ctr:      ccontainer.NewCContainer[world.Engine](nil),
		storeCtr: ccontainer.NewCContainer[*blockStoreAuthority](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "CDN world controller: "+c.conf.GetEngineId())
}

// ownsBlockStore reports whether this controller holds the CDN transport and
// therefore the LookupBlockStore authority for its Space.
func (c *Controller) ownsBlockStore() bool {
	return c.conf.GetSuppliedBlockStoreId() == ""
}

// newBlockStore builds the Release World block read path.
//
// With supplied_block_store_id set the reads traverse that bus block store and
// no CDN transport is opened here; the root pointer is still fetched so the
// mount can build its world head.
func (c *Controller) newBlockStore(ctx context.Context) (cdn_bstore.RootBlockStore, func(), error) {
	if suppliedID := c.conf.GetSuppliedBlockStoreId(); suppliedID != "" {
		suppliedStore, _, suppliedRef, err := block_store.ExLookupFirstBlockStore(ctx, c.b, suppliedID, false, nil)
		if err != nil {
			return nil, nil, err
		}
		store, err := cdn_bstore.NewSuppliedBlockStore(cdn_bstore.SuppliedOptions{
			CdnBaseURL: c.conf.GetCdnBaseUrl(),
			SpaceID:    c.conf.GetSpaceId(),
			HttpClient: http.DefaultClient,
			Store:      suppliedStore,
		})
		if err != nil {
			suppliedRef.Release()
			return nil, nil, err
		}
		return store, suppliedRef.Release, nil
	}

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
	if c.ownsBlockStore() {
		storeID := c.conf.GetSpaceId()
		if c.conf.GetEngineId() == releaseWorldEngineID {
			storeID = ReleaseBlockStoreID
		}
		authority := newBlockStoreAuthority(block_store.NewStore(storeID, store))
		c.storeCtr.SetValue(authority)
		defer func() {
			authority.withdraw()
			c.storeCtr.SetValue(nil)
			authority.wait()
		}()
	}
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
	switch dir := di.GetDirective().(type) {
	case world.LookupWorldEngine:
		if id := dir.LookupWorldEngineID(); id != "" && id != c.conf.GetEngineId() {
			return nil, nil
		}
		return directive.R(world.NewWorldEngineResolver(c))
	case block_store.LookupBlockStore:
		// A supplied-store mount reads through another owner's block store, so
		// answering here would publish a second provider for the same id.
		if !c.ownsBlockStore() {
			return nil, nil
		}
		id := dir.LookupBlockStoreId()
		if id != "" && id != c.conf.GetSpaceId() &&
			(id != ReleaseBlockStoreID || c.conf.GetEngineId() != releaseWorldEngineID) {
			return nil, nil
		}
		return directive.R(&blockStoreResolver{ctr: c.storeCtr}, nil)
	default:
		return nil, nil
	}
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
