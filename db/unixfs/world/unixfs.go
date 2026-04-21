package unixfs_world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

// LookupFsOp performs the lookup operation for the fs op types.
func LookupFsOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case FsInitOpId:
		return &FsInitOp{}, nil
	case FsCopyOpId:
		return &FsCopyOp{}, nil
	case FsMknodOpId:
		return &FsMknodOp{}, nil
	case FsSetModTimestampOpId:
		return &FsSetModTimestampOp{}, nil
	case FsSetPermissionsOpId:
		return &FsSetPermissionsOp{}, nil
	case FsTruncateOpId:
		return &FsTruncateOp{}, nil
	case FsRemoveOpId:
		return &FsRemoveOp{}, nil
	case FsRenameOpId:
		return &FsRenameOp{}, nil
	case FsSymlinkOpId:
		return &FsSymlinkOp{}, nil
	case FsWriteAtOpId:
		return &FsWriteAtOp{}, nil
	case FsMknodWithContentOpId:
		return &FsMknodWithContentOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupFsOp

// LookupFsType attempts to lookup the FS type of the world object.
// If unset, defaults to FS_NODE.
// Checks that the type ID is recognized.
func LookupFsType(ctx context.Context, ws world.WorldState, objKey string) (FSType, bool, error) {
	ot, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return 0, false, err
	}
	if ot == "" {
		return FSType_FSType_FS_NODE, false, nil
	}
	ft, err := TypeIDToFSType(ot)
	return ft, true, err
}

// ValidateOrCreateFs creates or checks a reference to a Unixfs.
// fsRef can be nil to create a new FS.
// returns the root ref, typeID, and error
func ValidateOrCreateFs(
	ctx context.Context,
	accessState world.AccessWorldStateFunc,
	fsType FSType,
	fsRef *bucket.ObjectRef,
	ts *timestamp.Timestamp,
) (*bucket.ObjectRef, string, error) {
	// check fsRef
	if !fsRef.GetEmpty() {
		fsRef = nil
	} else if err := fsRef.Validate(); err != nil {
		return nil, "", err
	}

	var err error
	var nroot block.Block
	var nrootTypeID string
	fsRef, err = world.AccessObject(ctx, accessState, fsRef, func(bcs *block.Cursor) error {
		currBlk, _ := bcs.GetBlock()
		if bcs.GetRef().GetEmpty() && currBlk == nil {
			// create new root
			nroot, nrootTypeID, err = NewFSRootWithType(fsType, unixfs.NewFSCursorNodeType_Dir(), ts)
			if err == nil {
				bcs.SetBlock(nroot, true)
			}
		} else {
			nroot, nrootTypeID, err = UnmarshalFSRootWithType(ctx, bcs, fsType)
			if err == nil && nroot == nil {
				err = block.ErrNotFound
			}
		}
		if err == nil {
			type validator interface {
				// Validate validates the block.
				Validate() error
			}
			v, ok := nroot.(validator)
			if ok {
				err = v.Validate()
			}
		}
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return fsRef, nrootTypeID, nil
}

// AccessUnixfsObject attempts to access a unixfs world object with multiple
// callbacks for each of the possible types of UnixFS objects.
//
// Returns an error if the given type was not handled.
func AccessUnixfsObject(
	ctx context.Context,
	objectHandle world.ObjectState,
	update bool,
	fsType FSType,
	cbFsTree func(ftree *unixfs_block.FSTree) error,
	cbHostVolume func(hv *unixfs_block.FSHostVolume) error,
) (*bucket.ObjectRef, bool, error) {
	return world.AccessObjectState(ctx, objectHandle, update, func(bcs *block.Cursor) error {
		switch fsType {
		case FSType_FSType_FS_NODE:
			if cbFsTree == nil {
				return errors.Wrap(ErrInvalidFSType, fsType.String())
			}
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			return cbFsTree(ftree)
		case FSType_FSType_FS_OBJECT:
			// TODO: handle FS_OBJECT
			return errors.Wrap(ErrInvalidFSType, fsType.String())
		case FSType_FSType_FS_HOST_VOLUME:
			if cbHostVolume == nil {
				return errors.Wrap(ErrInvalidFSType, fsType.String())
			}

			hv, err := unixfs_block.UnmarshalFSHostVolume(ctx, bcs)
			if err != nil {
				return err
			}
			return cbHostVolume(hv)
		default:
			return errors.Wrap(ErrInvalidFSType, fsType.String())
		}
	})
}
