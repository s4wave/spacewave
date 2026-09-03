package s4wave_device_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// DeviceTypeID is the type identifier for Spacewave-managed Device objects.
const DeviceTypeID = s4wave_device.DeviceTypeID

// DeviceType is the ObjectType for Spacewave-managed Device objects.
var DeviceType = objecttype.NewObjectType(DeviceTypeID, deviceFactory)

// ComputersDashboardTypeID is the type identifier for Computers dashboard objects.
const ComputersDashboardTypeID = s4wave_device.ComputersDashboardTypeID

// ComputersDashboardType is the ObjectType for Computers dashboard objects.
var ComputersDashboardType = objecttype.NewObjectType(ComputersDashboardTypeID, deviceReadOnlyFactory)

func deviceReadOnlyFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	return nil, func() {}, nil
}

func deviceFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	var state *s4wave_device.Device
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_device.UnmarshalDevice(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_device.NewDeviceResource(le, b, engine, ws, objectKey, state)
	return resource.GetMux(), func() {}, nil
}
