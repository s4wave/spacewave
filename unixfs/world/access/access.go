package unixfs_world_access

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
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
	fsRc   *refcount.RefCount[*unixfs.FS]
}

// newController constructs the controller with a bus controller.
func newController(base *bus.BusController[*Config]) (*Controller, error) {
	senderPeerID, err := base.GetConfig().ParsePeerID()
	if err != nil {
		return nil, err
	}
	ctrl := &Controller{BusController: base, sender: senderPeerID}
	ctrl.errCtr = ccontainer.NewCContainer[*error](nil)
	ctrl.fsRc = refcount.NewRefCount(nil, nil, ctrl.errCtr, ctrl.resolveFs)
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
	valProm, valRef, err := c.fsRc.WaitWithReleased(ctx, released)
	if err != nil {
		return nil, nil, err
	}
	val, err := valProm.Await(ctx)
	if err != nil {
		valRef.Release()
		return nil, nil, err
	}
	rootRef, err := val.AddRootReference(ctx)
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
func (c *Controller) resolveFs(ctx context.Context, released func()) (*unixfs.FS, func(), error) {
	eng, engRelease, err := c.resolveWorldEngine(ctx, released)
	if err != nil {
		return nil, nil, err
	}

	conf := c.GetConfig()
	fs, err := unixfs_world.BuildFSFromUnixfsRef(
		ctx,
		c.GetLogger(),
		world.NewEngineWorldState(ctx, eng, true),
		c.sender,
		conf.GetFsRef(),
		false,
		!conf.GetDisableWatchChanges(),
		conf.GetTimestamp().ToTime(),
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
