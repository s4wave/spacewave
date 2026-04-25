package space_sobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
)

// ControllerID is the controller id.
const ControllerID = "space/sobject"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "loads controllers for space shared objects"

// Controller is the space sobject controller.
type Controller struct {
	*bus.BusController[*Config]
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
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case sobject.MountSharedObjectBody:
		if dir.MountSharedObjectBodyType() == space.SpaceBodyType {
			return c.resolveMountSharedObjectBody(dir)
		}
	}
	return nil, nil
}

// resolveMountSharedObjectBody builds the space sobject body resolver.
func (c *Controller) resolveMountSharedObjectBody(dir sobject.MountSharedObjectBody) ([]directive.Resolver, error) {
	return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (space.MountSharedObjectBodyValue, func(), error) {
		mountRef := dir.MountSharedObjectBodyRef()
		engineID := space.SpaceEngineId(mountRef)
		conf := &sobject_world_engine.Config{
			// Refs and engine id
			EngineId: engineID,
			Ref:      mountRef,

			// Copy fields from config
			Verbose:           c.GetConfig().GetVerbose(),
			ProcessOpsBackoff: c.GetConfig().GetProcessOpsBackoff().CloneVT(),
		}
		if err := conf.Validate(); err != nil {
			return nil, nil, err
		}

		// mount the shared object first
		so, soRef, err := sobject.ExMountSharedObject(ctx, c.GetBus(), mountRef, false, released)
		if err != nil {
			return nil, nil, err
		}

		ctrl, _, ref, err := sobject_world_engine.StartEngineWithConfig(ctx, c.GetBus(), conf, released)
		if err != nil {
			soRef.Release()
			return nil, nil, err
		}

		eng, err := ctrl.GetWorldEngine(ctx)
		if err != nil {
			ref.Release()
			soRef.Release()
			return nil, nil, err
		}

		// get bucket ID and volume ID from the shared object's block store
		bucketID := so.GetBlockStore().GetID()
		volumeID := so.GetBlockStore().GetID()

		// construct the space body
		body := NewSpaceBody(mountRef, engineID, bucketID, volumeID, so, eng)

		// construct the mount value with the space body
		ret := sobject.NewMountSharedObjectBodyValue(mountRef, space.SpaceBodyType, so, body)

		return ret, func() {
			ref.Release()
			soRef.Release()
		}, nil
	}), nil)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
