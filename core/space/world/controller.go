package space_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// Controller serves the World engine of a Space on the bus: it mounts the
// Space's SharedObject, runs its Adapter, acknowledges the edits this device
// holds, keeps the trimming roster, orders the Space as its main device, and
// resolves LookupWorldEngine directives for its engine ID.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// bus mounts the SharedObject and resolves operations.
	bus bus.Bus
	// conf is the controller config.
	conf *Config
	// sfs builds the World block transformers.
	sfs *block_transform.StepFactorySet
	// opTypes resolves the operation types before the bus.
	opTypes OpTypes
	// adapterCtr holds the adapter of the running execution.
	adapterCtr *ccontainer.CContainer[*Adapter]
}

// NewController returns the controller of conf's Space World.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config, sfs *block_transform.StepFactorySet, opTypes OpTypes) *Controller {
	return &Controller{
		le:         le.WithField("engine-id", conf.GetEngineId()),
		bus:        b,
		conf:       conf,
		sfs:        sfs,
		opTypes:    opTypes,
		adapterCtr: ccontainer.NewCContainer[*Adapter](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "space world engine: "+c.conf.GetEngineId())
}

// Execute mounts the SharedObject and serves its World until ctx ends.
func (c *Controller) Execute(rctx context.Context) error {
	// Mount the SharedObject.
	ctx, cancel := context.WithCancel(rctx)
	defer cancel()
	so, soRef, err := sobject.ExMountSharedObject(ctx, c.bus, c.conf.GetRef(), false, cancel)
	if err != nil {
		return err
	}
	defer soRef.Release()

	// Build the adapter.
	a, err := NewAdapter(c.le, so, AdapterOptions{
		Bus:            c.bus,
		StepFactorySet: c.sfs,
		EngineID:       c.conf.GetEngineId(),
		OpTypes:        c.opTypes,
		DisableLookup:  c.conf.GetDisableLookup(),
		InitWorldOp:    c.conf.GetInitWorldOp(),
		Verbose:        c.conf.GetVerbose(),
	})
	if err != nil {
		return err
	}

	// Acknowledge, keep the roster and order as the main device while
	// mounted. Each stops alone on failure, as the engine keeps serving.
	background := func(what string, fn func() error) {
		go func() {
			if err := fn(); err != nil && ctx.Err() == nil {
				c.le.WithError(err).Warn("stopped " + what)
			}
		}()
	}
	background("acknowledging edits", func() error { return sobject.Acknowledge(ctx, so, a.Acknowledge) })
	if roster, ok := so.(sobject.RosterHost); ok {
		background("maintaining the trimming roster", func() error { return sobject.MaintainRoster(ctx, so, roster) })
	}
	if mainDevice, ok := so.(sobject.MainDevice); ok {
		background("ordering edits as the main device", func() error { return sobject.Sequence(ctx, so, mainDevice.SequenceOperations) })
	}

	// Serve the adapter, publishing it for the engine lookups.
	c.adapterCtr.SetValue(a)
	defer c.adapterCtr.SetValue(nil)
	err = a.Execute(ctx)
	if rctx.Err() != nil {
		return nil
	}
	return err
}

// HandleDirective resolves LookupWorldEngine for the engine ID.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Answer only a lookup of this engine.
	dir, ok := di.GetDirective().(world.LookupWorldEngine)
	engineID := c.conf.GetEngineId()
	if !ok || engineID == "" {
		return nil, nil
	}
	if id := dir.LookupWorldEngineID(); id != "" && id != engineID {
		return nil, nil
	}
	return directive.R(world.NewWorldEngineResolver(c))
}

// GetWorldEngine waits for the served engine.
func (c *Controller) GetWorldEngine(ctx context.Context) (world.Engine, error) {
	eng, release, err := c.AcquireWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	release()
	return eng, nil
}

// AcquireWorldEngine waits for the served engine and holds it open until
// release is called.
func (c *Controller) AcquireWorldEngine(ctx context.Context) (world.Engine, func(), error) {
	// Wait for the running adapter.
	a, err := c.adapterCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, nil, err
	}

	// Hold its engine.
	engine, release, err := a.AcquireEngine(ctx)
	if err != nil {
		return nil, nil, err
	}
	return engine, release, nil
}

// WaitWorldEngineReplaced waits until eng no longer serves.
func (c *Controller) WaitWorldEngineReplaced(ctx context.Context, eng world.Engine) error {
	a, err := c.adapterCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	engine, _ := eng.(*Engine)
	return a.WaitEngineReplaced(ctx, engine)
}

// Close releases the controller's resources.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ world.ReplacingController = (*Controller)(nil)
