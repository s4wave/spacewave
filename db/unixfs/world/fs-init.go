package unixfs_world

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FsInit initializes a new fs in a world.
// Returns any error.
func FsInit(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	fsType FSType,
	fsRef *bucket.ObjectRef,
	overwrite bool,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	initOp := NewFsInitOp(
		objKey,
		fsType,
		fsRef,
		overwrite,
		ts,
	)
	return ws.ApplyWorldOp(ctx, initOp, sender)
}

// NewFsInitOp constructs a new FsInitOp block. fsRef may be nil.
func NewFsInitOp(
	objKey string,
	fsType FSType,
	fsRef *bucket.ObjectRef,
	overwrite bool,
	ts time.Time,
) *FsInitOp {
	return &FsInitOp{
		ObjectKey:   objKey,
		FsType:      fsType,
		FsRef:       fsRef,
		FsOverwrite: overwrite,
		Timestamp:   unixfs_block.ToTimestamp(ts, true),
	}
}

// Validate checks the op cursorily.
func (o *FsInitOp) Validate() error {
	// Check the object key, timestamp, filesystem type and reference.
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if err := o.GetFsRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateOrCreateFs validates or creates the fs.
//
// Returns the root ref, type ID string, and error.
func (o *FsInitOp) ValidateOrCreateFs(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	ts *timestamp.Timestamp,
) (*bucket.ObjectRef, string, error) {
	// create / validate the objectref for the fs
	return ValidateOrCreateFs(
		ctx,
		access,
		o.GetFsType(),
		o.GetFsRef(),
		ts,
	)
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsInitOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// create / validate the objectref for the fs
	fsRef, fsTypeID, err := o.ValidateOrCreateFs(ctx, worldHandle.AccessWorldState, o.GetTimestamp())
	if err != nil {
		return false, err
	}

	// check whether the fs object exists
	objKey := o.GetObjectKey()
	objState, exists, err := worldHandle.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return false, err
	}
	if exists {
		if o.GetFsOverwrite() {
			_, err = objState.SetRootRef(ctx, fsRef)
			return false, err
		} else {
			return false, world.ErrObjectExists
		}
	} else {
		// create the fs object
		var createdObject world.ObjectState
		createdObject, err = worldHandle.CreateObject(ctx, objKey, fsRef)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return false, err
		}
	}

	// create the types reference
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, fsTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *FsInitOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// create / validate the objectref for the fs
	fsRef, _, err := o.ValidateOrCreateFs(ctx, objectHandle.AccessWorldState, o.GetTimestamp())
	if err != nil {
		return false, err
	}

	// update the object
	_, err = objectHandle.SetRootRef(ctx, fsRef)
	return false, err
}

// _ is a type assertion
var _ world.Operation = (*FsInitOp)(nil)
