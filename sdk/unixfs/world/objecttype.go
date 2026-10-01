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
// Returns a FSHandleResource for the world object.
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
	// Require a World state to construct the object resource.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Look up the FS node type from the World state.
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	// Open a read-write or read-only FS cursor for the object.
	var fsCursor *unixfs_world.FSCursor
	if !ws.GetReadOnly() {
		fsCursor, _ = unixfs_world.NewFSCursorWithWriterContext(ctx, le, ws, objectKey, fsType, "")
	} else {
		fsCursor = unixfs_world.NewFSCursorWithContext(ctx, le, ws, objectKey, fsType, nil, false)
	}

	// Construct the FS handle resource and its cleanup.
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, nil, err
	}

	// Wrap the FS handle in an object resource.
	resource := resource_unixfs.NewFSHandleObjectResource(
		le,
		fsh,
		nil,
		ws,
		objectKey,
		fsType,
		nil,
	)

	// Return the resource's SRPC mux with its cleanup.
	cleanup := func() {
		fsh.Release()
		fsCursor.Release()
	}

	return resource.GetMux(), cleanup, nil
}
