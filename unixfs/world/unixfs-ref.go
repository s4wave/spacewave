package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/sirupsen/logrus"
)

// BuildFSFromUnixfsRef builds a unixfs FS from a Unixfs ref.
//
// if mkdirPath is set, creates the Path in the ref if not exists.
// sender is the peer ID to use for write transactions.
// if sender is empty, the writer will be nil (read-only FS).
func BuildFSFromUnixfsRef(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
	ref *UnixfsRef,
	mkdirPath bool,
	watchChanges bool,
	ts time.Time,
) (*unixfs.FS, error) {
	// lookup the object
	fsCursor, err := FollowUnixfsRef(ctx, le, ws, ref, sender, watchChanges && !mkdirPath)
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

	// if we should mkdirPath, ensure the prefix exists first.
	if mkdirPath {
		baseFs := unixfs.NewFS(ctx, le, fsCursor, nil)
		baseFsh, err := baseFs.AddRootReference(ctx)
		if err != nil {
			baseFs.Release()
			return nil, err
		}
		err = baseFsh.MkdirAll(
			ctx,
			unixfs.JoinPath(prefixPath),
			unixfs_block.DefaultPermissions(unixfs_block.NodeType_NodeType_DIRECTORY),
			ts,
		)
		baseFsh.Release()
		baseFs.Release()
		if err != nil {
			return nil, err
		}

		// rebuild the fs cursor
		fsCursor.Release()
		fsCursor, err = FollowUnixfsRef(ctx, le, ws, ref, sender, watchChanges)
		if err != nil {
			return nil, err
		}
	}

	fs := unixfs.NewFS(ctx, le, fsCursor, prefixPath)
	return fs, nil
}

// FollowUnixfsRef builds a fs cursor based on a Unixfs ref.
//
// NOTE: ignores the Path field!
// if sender is empty, the writer will be nil.
func FollowUnixfsRef(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	ref *UnixfsRef,
	sender peer.ID,
	watchChanges bool,
) (*FSCursor, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	objKey := ref.GetObjectKey()
	fsType := ref.GetFsType()
	if fsType == 0 {
		// determine based on types
		typeID, err := world_types.GetObjectType(ctx, ws, objKey)
		if err != nil {
			return nil, err
		}
		fsType, err = TypeIDToFSType(typeID)
		if err != nil {
			return nil, err
		}
		// fails if the type == UNKNOWN
		if err := fsType.Validate(false); err != nil {
			return nil, err
		}
	}

	var writer unixfs.FSWriter
	if len(sender) != 0 {
		writer = NewFSWriter(ws, objKey, fsType, sender)
	}
	return NewFSCursor(le, ws, objKey, fsType, writer, watchChanges), nil
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
