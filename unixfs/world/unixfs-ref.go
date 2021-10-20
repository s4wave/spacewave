package unixfs_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/sirupsen/logrus"
)

// BuildFSFromUnixfsRef builds a unixfs FS from a Unixfs ref.
//
// if sender is empty, the writer will be nil.
func BuildFSFromUnixfsRef(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	ref *UnixfsRef,
	sender peer.ID,
) (*unixfs.FS, error) {
	fsCursor, err := FollowUnixfsRef(ctx, ws, ref, sender)
	if err != nil {
		return nil, err
	}

	// apply prefix if necessary
	var prefixPath []string
	refPath := ref.GetPath()
	if len(refPath.GetNodes()) != 0 {
		if err := refPath.Validate(); err != nil {
			return nil, err
		}
		prefixPath = unixfs_block.PathsToStringSlices(refPath)[0]
	}

	return unixfs.NewFS(ctx, le, fsCursor, prefixPath), nil
}

// FollowUnixfsRef builds a fs cursor based on a Unixfs ref.
//
// NOTE: ignores the Path field!
// if sender is empty, the writer will be nil.
func FollowUnixfsRef(
	ctx context.Context,
	ws world.WorldState,
	ref *UnixfsRef,
	sender peer.ID,
) (*FSCursor, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	objKey := ref.GetObjectKey()
	fsType := ref.GetFsType()
	if fsType == 0 {
		// determine based on types
		ts := world_types.NewTypesState(ctx, ws)
		typeID, err := ts.GetObjectType(objKey)
		if err != nil {
			return nil, err
		}
		fsType, err = TypeIDToFSType(typeID)
		if err != nil {
			return nil, err
		}
		if err := fsType.Validate(false); err != nil {
			return nil, err
		}
	}

	var writer unixfs.FSWriter
	if len(sender) != 0 {
		writer = NewFSWriter(ws, objKey, fsType, sender)
	}
	return NewFSCursor(ws, objKey, fsType, writer), nil
}

// Validate checks the unixfs ref.
func (u *UnixfsRef) Validate() error {
	if len(u.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := u.GetFsType().Validate(true); err != nil {
		return err
	}
	return nil
}

// Clone copies the unixfs ref.
func (u *UnixfsRef) Clone() *UnixfsRef {
	if u == nil {
		return nil
	}
	return &UnixfsRef{
		ObjectKey: u.GetObjectKey(),
		FsType:    u.GetFsType(),
		Path:      u.GetPath(),
	}
}
