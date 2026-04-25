package space_unixfs

import (
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// BuildFSHandle constructs a projected FSHandle for one space.
func BuildFSHandle(
	le *logrus.Entry,
	ws world.WorldState,
	sessionIdx uint32,
	sharedObjectID string,
) (*unixfs.FSHandle, error) {
	cursor := NewFSCursor(le, ws, sessionIdx, sharedObjectID)
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	return handle, nil
}
