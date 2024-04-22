package unixfs_world_access

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "hydra/unixfs/world/access"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "access world-backed unixfs"

// Controller is the unixfs world access controller.
type Controller struct {
	*bus.BusController[*Config]
	sender peer.ID
	errCtr *ccontainer.CContainer[*error]
	fsRc   *refcount.RefCount[*unixfs.FSHandle]
}

// newController constructs the controller with a bus controller.
func newController(base *bus.BusController[*Config]) (*Controller, error) {
	senderPeerID, err := base.GetConfig().ParsePeerID()
	if err != nil {
		return nil, err
	}
	ctrl := &Controller{BusController: base, sender: senderPeerID}
	ctrl.errCtr = ccontainer.NewCContainer[*error](nil)
	// note: keep the handle if we have zero references and the context is not canceled.
	ctrl.fsRc = refcount.NewRefCount(nil, true, nil, ctrl.errCtr, ctrl.resolveFs)
	return ctrl, nil
}

// NewController constructs the controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	return newController(bus.NewBusController(le, b, conf, ControllerID, Version, controllerDescrip))
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		newController,
	)
}

// NewAccessUnixFSFunc builds a new AccessUnixFSFunc with a Controller config and bus.
// When Access is called, executes the UnixFS Access controller with the given config.
// Waits for the controller to be ready and uses the Access function on the controller.
// Returns the resulting FSHandle.
// Calling the release function releases the handle to the LoadControllerWithConfig.
func NewAccessUnixFSFunc(b bus.Bus, conf *Config) unixfs_access.AccessUnixFSFunc {
	return func(rctx context.Context, rreleased func()) (*unixfs.FSHandle, func(), error) {
		ctx, ctxCancel := context.WithCancel(rctx)
		released := func() {
			ctxCancel()
			if rreleased != nil {
				rreleased()
			}
		}
		accessCtrlInter, _, accessCtrlRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(conf),
			released,
		)
		if err != nil {
			return nil, nil, err
		}
		ctrl, ok := accessCtrlInter.(*Controller)
		if !ok {
			ctxCancel()
			accessCtrlRef.Release()
			return nil, nil, errors.New("unexpected controller type for unixfs/world/access")
		}
		handle, rel, err := ctrl.AccessUnixFS(ctx, released)
		if err != nil {
			ctxCancel()
			accessCtrlRef.Release()
			return nil, nil, err
		}
		return handle, func() {
			ctxCancel()
			rel()
			accessCtrlRef.Release()
			released()
		}, nil
	}
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.fsRc.SetContext(ctx)
	defer c.fsRc.SetContext(nil)

	rerr, err := c.errCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	return *rerr
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case unixfs_access.AccessUnixFS:
		return directive.R(c.ResolveAccessUnixFS(ctx, di, d), nil)
	}
	return nil, nil
}

// ResolveAccessUnixFS resolves an AccessUnixFS directive if the fs id matches.
func (c *Controller) ResolveAccessUnixFS(
	ctx context.Context,
	di directive.Instance,
	d unixfs_access.AccessUnixFS,
) directive.Resolver {
	fsID := d.AccessUnixFSID()
	if c.GetConfig().GetFsId() != fsID {
		return nil
	}
	return directive.NewValueResolver([]unixfs_access.AccessUnixFSValue{c.AccessUnixFS})
}

// AccessUnixFS accesses the filesystem.
func (c *Controller) AccessUnixFS(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
	valProm, valRef := c.fsRc.WaitWithReleased(ctx, released)
	val, err := valProm.Await(ctx)
	if err != nil {
		valRef.Release()
		return nil, nil, err
	}
	rootRef, err := val.Clone(ctx)
	if err != nil {
		valRef.Release()
		return nil, nil, err
	}
	return rootRef, func() {
		rootRef.Release()
		valRef.Release()
	}, nil
}

// resolveFs resolves building the fs.
func (c *Controller) resolveFs(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
	eng, engRelease, err := c.resolveWorldEngine(ctx, released)
	if err != nil {
		return nil, nil, err
	}

	conf := c.GetConfig()
	fs, err := unixfs_world.BuildFSFromUnixfsRef(
		ctx,
		c.GetLogger(),
		world.NewEngineWorldState(eng, true),
		c.sender,
		conf.GetFsRef(),
		false,
		!conf.GetDisableWatchChanges(),
		conf.GetTimestamp().AsTime(),
	)
	if err != nil {
		engRelease()
		return nil, nil, err
	}

	return fs, func() {
		fs.Release()
		engRelease()
	}, nil
}

// resolveWorldEngine resolves looking up the world engine on the bus.
func (c *Controller) resolveWorldEngine(ctx context.Context, released func()) (world.Engine, func(), error) {
	engh, _, engRef, err := world.ExLookupWorldEngine(ctx, c.GetBus(), false, c.GetConfig().GetEngineId(), released)
	if err != nil {
		return nil, nil, err
	}
	return engh, engRef.Release, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
