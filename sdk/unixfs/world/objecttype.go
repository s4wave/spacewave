package s4wave_unixfs_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// UnixFSTypeID is the object type ID for UnixFS fs-node objects.
const UnixFSTypeID = unixfs_world.FSNodeTypeID

// UnixFSType is the ObjectType for UnixFS objects.
// Returns a FSHandleResource which mirrors the Hydra FSHandle interface.
var UnixFSType = objecttype.NewObjectType(UnixFSTypeID, UnixFSFactory)

// UnixFSFactory creates a FSHandleResource from a world object.
func UnixFSFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	_ world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	var fsCursor *unixfs_world.FSCursor
	if !ws.GetReadOnly() {
		fsCursor, _ = unixfs_world.NewFSCursorWithWriter(ctx, le, ws, objectKey, fsType, "")
	} else {
		fsCursor = unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false)
	}

	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, nil, err
	}

	resource := resource_unixfs.NewFSHandleObjectResource(
		fsh,
		nil,
		ws,
		objectKey,
		fsType,
		nil,
	)

	cleanup := func() {
		fsh.Release()
	}

	return resource.GetMux(), cleanup, nil
}
