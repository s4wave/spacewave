// Package s4wave_vm_world registers VM ObjectTypes in the Space World.
package s4wave_vm_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/s4wave/spacewave/db/world"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// VmV86Type is the ObjectType for spacewave/vm/v86 objects.
var VmV86Type = objecttype.NewObjectType(s4wave_vm.VmV86TypeID, vmV86Factory)

// V86ImageType is the ObjectType for spacewave/vm/image/v86 objects.
// V86Image is metadata-only; block state is read through the objectState prop.
var V86ImageType = objecttype.NewObjectType(s4wave_vm.V86ImageTypeID, v86ImageReadOnlyFactory)

// v86ImageReadOnlyFactory is a minimal factory for the read-only V86Image type.
func v86ImageReadOnlyFactory(
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

// vmV86Factory creates a V86 resource with PersistentExecutionService and V86fsService.
func vmV86Factory(
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

	// Create v86fs server with mount resolver that resolves graph edges to FSHandle.
	v86fsServer := unixfs_v86fs.NewServer(func(ctx context.Context, name string) (*unixfs.FSHandle, error) {
		return resolveV86Mount(ctx, ws, objectKey, name)
	})

	// Pre-populate the v86fs dynamic mount table from V86Config.Mounts so the
	// guest learns about workspace/home/etc. mounts via MOUNT_NOTIFY frames
	// when the v86fs session joins.
	mountCleanup, err := registerV86ConfigMounts(ctx, ws, objectKey, v86fsServer)
	if err != nil {
		return nil, nil, err
	}

	resource := newV86Resource(objectKey, ws, b)
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		if err := s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource); err != nil {
			return err
		}
		return unixfs_v86fs.SRPCRegisterV86FsService(mux, v86fsServer)
	})
	return mux, mountCleanup, nil
}
