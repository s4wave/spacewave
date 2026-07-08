package manifest_fetch_world

import (
	"context"
	"net/http"
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/manifest/fetch/world"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller fetches Manifests via world lookups.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// fetchManifestIdRe is the parsed regex to filter manifest by.
	// if nil, accepts any
	fetchManifestIdRe *regexp.Regexp
	// engine is the CDN-backed world engine exposed when CDN fields are set.
	engine *cdn_sharedobject.WorldEngine
	// ctr publishes the CDN-backed world engine to LookupWorldEngine callers.
	ctr *ccontainer.CContainer[world.Engine]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	manifestIdRe, _ := conf.ParseFetchManifestIdRe()
	return &Controller{
		le:                le,
		bus:               bus,
		conf:              conf,
		fetchManifestIdRe: manifestIdRe,
		ctr:               ccontainer.NewCContainer[world.Engine](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"fetches manifests via world",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	if c.conf.GetCdnSpaceId() == "" {
		return nil
	}

	pointerTTL, _ := c.conf.ParsePointerTTLDur()
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: c.conf.GetCdnBaseUrl(),
		SpaceID:    c.conf.GetCdnSpaceId(),
		HttpClient: http.DefaultClient,
		PointerTTL: pointerTTL,
	})
	if err != nil {
		return err
	}
	defer store.Close()

	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    c.conf.GetCdnSpaceId(),
		BlockStore: store,
	})
	if err != nil {
		return err
	}

	engine, err := cdn_sharedobject.NewWorldEngine(ctx, c.le, c.bus, so, bldr_manifest_world.LookupOp)
	if err != nil {
		return err
	}
	c.engine = engine
	c.ctr.SetValue(engine.Engine)
	c.le.WithField("engine-id", c.conf.GetEngineId()).Info("manifest CDN world engine ready")

	<-ctx.Done()
	engine.Release()
	c.engine = nil
	c.ctr.SetValue(nil)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, inst, d))
	case world.LookupWorldEngine:
		return c.resolveLookupWorldEngine(d)
	}
	return nil, nil
}

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) (directive.Resolver, error) {
	if c.fetchManifestIdRe != nil && dir.GetManifestId() != "" {
		if !c.fetchManifestIdRe.MatchString(dir.GetManifestId()) {
			return nil, nil
		}
	}

	return &fetchManifestResolver{c: c, dir: dir}, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	if c.engine != nil {
		c.engine.Release()
		c.engine = nil
		c.ctr.SetValue(nil)
	}
	return nil
}
func (c *Controller) resolveLookupWorldEngine(dir world.LookupWorldEngine) ([]directive.Resolver, error) {
	if c.conf.GetCdnSpaceId() == "" {
		return nil, nil
	}
	if id := dir.LookupWorldEngineID(); id != "" && id != c.conf.GetEngineId() {
		return nil, nil
	}
	return directive.R(world.NewWorldEngineResolver(c))
}

// GetWorldEngine waits for the CDN-backed manifest world engine.
func (c *Controller) GetWorldEngine(ctx context.Context) (world.Engine, error) {
	return c.ctr.WaitValue(ctx, nil)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
