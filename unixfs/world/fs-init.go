package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
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
	fsRefType FSType,
	overwrite bool,
	ts time.Time,
) error {
	// perform the fs init operation
	initOp := NewFsInitOp(
		objKey,
		fsType,
		fsRef,
		fsRefType,
		overwrite,
		ts,
	)
	_, _, err := ws.ApplyWorldOp(initOp, sender)
	return err
}

// FsInitOpId is the unixfs init operation id.
var FsInitOpId = "hydra/unixfs/init"

// NewFsInitOp constructs a new FsInitOp block.
// repoRef, worktreeArgs can be empty
func NewFsInitOp(
	objKey string,
	fsType FSType,
	fsRef *bucket.ObjectRef,
	fsRefType FSType,
	overwrite bool,
	ts time.Time,
) *FsInitOp {
	return &FsInitOp{
		ObjectKey:   objKey,
		FsType:      fsType,
		FsRef:       fsRef,
		FsRefType:   fsRefType,
		FsOverwrite: overwrite,
		Timestamp:   unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsInitOpBlock constructs a new FsInitOp block.
func NewFsInitOpBlock() block.Block {
	return &FsInitOp{}
}

// Validate performs cursory checks on the op.
func (o *FsInitOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := o.GetFsRefType().Validate(true); err != nil {
		return err
	}
	if err := o.GetFsRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsInitOp) GetOperationTypeId() string {
	return FsInitOpId
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
		o.GetFsRefType(),
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

	// check if exists
	objKey := o.GetObjectKey()
	objState, exists, err := worldHandle.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if exists {
		if o.GetFsOverwrite() {
			_, err = objState.SetRootRef(fsRef)
			return false, err
		} else {
			return false, world.ErrObjectExists
		}
	} else {
		// create the fs object
		_, err = worldHandle.CreateObject(objKey, fsRef)
		if err != nil {
			return false, err
		}
	}

	// create the types reference
	types := world_types.NewTypesState(ctx, worldHandle)
	if err := types.SetObjectType(objKey, fsTypeID); err != nil {
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
	_, err = objectHandle.SetRootRef(fsRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsInitOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsInitOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsInitOp)(nil))
