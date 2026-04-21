package unixfs_rpc

import (
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
)

// ReadAtSizeLimit is a hardcoded cap on how much data can be read with ReadAt
// in a single packet to avoid encoding too large of a packet in memory.
//
// This value may be changed in future and is applied server-side.
const ReadAtSizeLimit int64 = 256e7

// NewFSCursorChange converts a FSCursorChange into a RPC FSCursorChange.
func NewFSCursorChange(cursorHandleID uint64, ch *unixfs.FSCursorChange) *FSCursorChange {
	return &FSCursorChange{
		CursorHandleId: cursorHandleID,
		Released:       ch.Released,
		Offset:         ch.Offset,
		Size:           ch.Size,
	}
}

// ToFSCursorChange converts the FSCursorChange into a unixfs.FSCursorChange.
func (c *FSCursorChange) ToFSCursorChange() *unixfs.FSCursorChange {
	return &unixfs.FSCursorChange{
		Released: c.GetReleased(),
		Offset:   c.GetOffset(),
		Size:     c.GetSize(),
	}
}

// NewFSCursorDirent converts a FSCursorDirent into a RPC FSCursorDirent.
func NewFSCursorDirent(dirent unixfs.FSCursorDirent) *FSCursorDirent {
	return &FSCursorDirent{
		Name:     dirent.GetName(),
		NodeType: unixfs_block.FSCursorNodeTypeToNodeType(dirent),
	}
}

// GetIsDirectory returns if the node is a directory.
func (d *FSCursorDirent) GetIsDirectory() bool {
	return d.GetNodeType().GetIsDirectory()
}

// GetIsFile returns if the node is a regular file.
func (d *FSCursorDirent) GetIsFile() bool {
	return d.GetNodeType().GetIsFile()
}

// GetIsSymlink returns if the node is a symlink.
func (d *FSCursorDirent) GetIsSymlink() bool {
	return d.GetNodeType().GetIsSymlink()
}

// _ is a type assertion
var _ unixfs.FSCursorDirent = ((*FSCursorDirent)(nil))
