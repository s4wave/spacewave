package cdn_world_controller

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/core/sobject"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/world"
	rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the CDN world controller.
const ControllerID = "spacewave/cdn/world"

// Version is the version of the world implementation.
var Version = controller.MustParseVersion("0.0.1")

// unreachableAfterFailures is the number of consecutive failed pointer fetches
// after which a mounting Space is reported unreachable.
const unreachableAfterFailures = 2

// releaseWorldEngineID selects the shared Release World block-store authority.
const releaseWorldEngineID = "spacewave-release-world"

// ReleaseBlockStoreID identifies the block store shared by Release World
// metadata reads and release-manifest bucket reads.
const ReleaseBlockStoreID = "spacewave-release-cdn"

// Controller exposes a read-only CDN-backed world engine.
type Controller struct {
	// le records engine and CDN failures.
	le *logrus.Entry
	// b resolves the configured block store.
	b bus.Bus
	// conf is the immutable mount configuration.
	conf *Config
	// engine owns the active CDN cursor and refresh routine.
	engine *cdn_sharedobject.WorldEngine
	// ctr publishes the outcome of the latest mount attempt.
	ctr *ccontainer.CContainer[*mountState]
	// storeCtr publishes block-store authority while this controller owns it.
	storeCtr *ccontainer.CContainer[*blockStoreAuthority]
	// refresh coalesces invalidations under the active mount's lifetime and
	// retries failed pointer fetches with backoff.
	refresh *routine.RoutineContainer
	// refreshed publishes the outcome of the latest pointer fetch.
	refreshed *ccontainer.CContainer[*refreshResult]
}

// refreshResult is the outcome of one root pointer fetch.
type refreshResult struct {
	// err is the fetch failure, or nil after a successful fetch.
	err error
}

// mountState is the outcome of a mount attempt: a readable engine, or the
// failure that left the Space unreadable.
type mountState struct {
	// engine is the world engine over the current readable head.
	engine world.Engine
	// err is the mount failure while no head is readable.
	err error
}

// NewController builds a new CDN world controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	c := &Controller{
		le:        le.WithField("engine-id", conf.GetEngineId()),
		b:         b,
		conf:      conf,
		ctr:       ccontainer.NewCContainer[*mountState](nil),
		storeCtr:  ccontainer.NewCContainer[*blockStoreAuthority](nil),
		refreshed: ccontainer.NewCContainer[*refreshResult](nil),
	}
	c.refresh = routine.NewRoutineContainerWithLogger(
		le,
		routine.WithRetry(&backoff.Backoff{}),
		routine.WithExitCb(func(err error) { c.refreshed.SetValue(&refreshResult{err: err}) }),
	)
	return c
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
	// Both read paths re-fetch the root pointer at the configured age.
	pointerTTL, _ := c.conf.ParsePointerTTLDur()

	// Reuse the configured authority when another bus owns the CDN store.
	if suppliedID := c.conf.GetSuppliedBlockStoreId(); suppliedID != "" {
		suppliedStore, _, suppliedRef, err := block_store.ExLookupFirstBlockStore(ctx, c.b, suppliedID, false, nil)
		if err != nil {
			return nil, nil, err
		}
		store, err := cdn_bstore.NewSuppliedBlockStore(cdn_bstore.SuppliedOptions{
			CdnBaseURL:         c.conf.GetCdnBaseUrl(),
			RootPointerBaseURL: c.conf.GetRootPointerBaseUrl(),
			SpaceID:            c.conf.GetSpaceId(),
			HttpClient:         http.DefaultClient,
			PointerTTL:         pointerTTL,
			Store:              suppliedStore,
		})
		if err != nil {
			suppliedRef.Release()
			return nil, nil, err
		}
		return store, suppliedRef.Release, nil
	}

	// Otherwise own the CDN transport and its durable writeback cache.
	store, releaseStore, err := cdn_bstore.NewCachedBlockStore(ctx, c.b, cdn_bstore.CachedBlockStoreOptions{
		CdnBaseURL:           c.conf.GetCdnBaseUrl(),
		RootPointerBaseURL:   c.conf.GetRootPointerBaseUrl(),
		SpaceID:              c.conf.GetSpaceId(),
		CacheBlockStoreID:    c.conf.GetCacheBlockStoreId(),
		PointerTTL:           pointerTTL,
		WritebackWindowBytes: c.conf.GetWritebackWindowBytes(),
		HttpClient:           http.DefaultClient,
	})
	return store, releaseStore, err
}

// Execute builds the CDN world engine and holds it until shutdown. A failed
// mount is published so lookups stop waiting on an unreachable Space.
func (c *Controller) Execute(ctx context.Context) error {
	err := c.mount(ctx)
	if err != nil && ctx.Err() == nil {
		c.ctr.SetValue(&mountState{err: err})
	}
	return err
}

// mount builds the CDN world engine and holds it until ctx is canceled.
func (c *Controller) mount(ctx context.Context) error {
	// Hold the backing store until the mount is withdrawn.
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

	// Represent the published CDN Space without creating authored state.
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    c.conf.GetSpaceId(),
		BlockStore: store,
	})
	if err != nil {
		return err
	}

	// The refresh routine owns pointer fetches, including the first one, so
	// mounting fetches the pointer once. While the CDN is unreachable the
	// routine retries and the store stays mounted, so cached blocks remain
	// readable. Lookups learn the Space is unreachable only after a retry also
	// fails, so one dropped request does not start the cached release.
	c.refreshed.SetValue(nil)
	c.refresh.SetRoutine(so.RefreshSnapshot)
	c.refresh.SetContext(ctx, false)
	defer c.refresh.ClearContext()
	var refreshed *refreshResult
	for failures := 1; ; failures++ {
		var err error
		refreshed, err = c.refreshed.WaitValueChange(ctx, refreshed, nil)
		if err != nil {
			return nil
		}
		if refreshed.err == nil {
			break
		}
		if failures >= unreachableAfterFailures {
			c.ctr.SetValue(&mountState{err: refreshed.err})
		}
	}

	// Wait for a readable head, then publish one engine until cancellation.
	for {
		previous := store.Pointer()
		engine, err := cdn_sharedobject.NewWorldEngine(ctx, c.le, c.b, so)
		if err == nil {
			c.engine = engine
			c.ctr.SetValue(&mountState{engine: engine.Engine})
			c.le.Info("CDN world engine ready")
			<-ctx.Done()
			c.refresh.ClearContext()
			engine.Release()
			c.engine = nil
			c.ctr.SetValue(nil)
			return nil
		}
		if !isMissingPublishedHead(err) {
			return err
		}
		c.le.WithError(err).Debug("CDN world engine waiting for published head")
		c.ctr.SetValue(&mountState{err: err})

		// Observe the store revision captured before the build, including a
		// publication that arrived while the engine was reading its head.
		if _, err := store.WaitPointer(ctx, previous); err != nil {
			return nil
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case rpc.LookupRpcService:
		if dir.LookupRpcServiceID() != WorldRefreshServiceID(c.conf.GetEngineId()) {
			return nil, nil
		}
		return directive.R(rpc.NewLookupRpcServiceResolver(c), nil)
	case world.LookupWorldEngine:
		if id := dir.LookupWorldEngineID(); id != "" && id != c.conf.GetEngineId() {
			return nil, nil
		}
		return directive.R(&worldEngineResolver{ctr: c.ctr}, nil)
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
	state, err := c.ctr.WaitValueWithValidator(ctx, func(state *mountState) (bool, error) {
		return state != nil && state.engine != nil, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return state.engine, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	if c.engine != nil {
		c.engine.Release()
		c.engine = nil
	}
	return nil
}

// isMissingPublishedHead identifies the retryable absence of a CDN root.
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
