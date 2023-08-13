package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/unixfs"
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
) (*unixfs.FSHandle, error) {
	// lookup the object
	fsCursor, err := FollowUnixfsRef(ctx, le, ws, ref, sender, watchChanges)
	if err != nil {
		return nil, err
	}

	// apply prefix if necessary
	refPath := ref.GetPath()
	prefixPath := refPath.GetNodes()
	if len(prefixPath) != 0 {
		if err := refPath.Validate(); err != nil {
			return nil, err
		}
	}

	// follow the path prefix
	return unixfs.NewFSHandleWithPrefix(ctx, fsCursor, prefixPath, mkdirPath, ts)
}

// FollowUnixfsRef builds a fs cursor based on a Unixfs ref.
//
// NOTE: ignores the Path field!
// if sender is empty the fs will be read-only.
// if watchChanges is nil the fs will be read-only.
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

	var fsw *FSWriter
	var writer unixfs.FSWriter
	if len(sender) != 0 && watchChanges {
		fsw = NewFSWriter(ws, objKey, fsType, sender)
		writer = fsw
	}

	// construct the fs cursor
	fsc := NewFSCursor(le, ws, objKey, fsType, writer, watchChanges)

	// we need the writer to wait until the FSCursor has processed the updated
	// revision of the world before returning from writes. pass the FSCursor to
	// the writer to set the additional wait function.
	if fsw != nil {
		fsw.SetConfirmFunc(fsc.WaitObjectRev)
	}

	return fsc, nil
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
