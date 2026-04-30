package cdn_world_controller

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver/v4"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the CDN world controller.
const ControllerID = "spacewave/cdn/world"

// Version is the version of the world implementation.
var Version = semver.MustParse("0.0.1")

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

// Execute builds the read-only CDN world engine and holds it until shutdown.
func (c *Controller) Execute(ctx context.Context) error {
	pointerTTL, _ := c.conf.ParsePointerTTLDur()
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: c.conf.GetCdnBaseUrl(),
		SpaceID:    c.conf.GetSpaceId(),
		HttpClient: http.DefaultClient,
		PointerTTL: pointerTTL,
	})
	if err != nil {
		return err
	}
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    c.conf.GetSpaceId(),
		BlockStore: store,
	})
	if err != nil {
		return err
	}
	engine, err := cdn_sharedobject.NewWorldEngine(ctx, c.le, c.b, so, space_world_optypes.LookupWorldOp)
	if err != nil {
		return err
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

// _ is a type assertion.
var (
	_ controller.Controller = (*Controller)(nil)
	_ world.Controller      = (*Controller)(nil)
)
