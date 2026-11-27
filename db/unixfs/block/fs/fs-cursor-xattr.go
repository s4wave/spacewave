package unixfs_block_fs

import (
	"context"

	"github.com/s4wave/spacewave/db/unixfs"
)

// GetXattrs returns the extended attributes for this node.
func (f *FSCursorOps) GetXattrs(ctx context.Context) ([]unixfs.FSXattr, error) {
	// Return no attributes for a released cursor or an empty attribute list.
	if f.CheckReleased() {
		return nil, nil
	}
	return f.fsTree.GetXattrs(), nil
}

// _ is a type assertion
var _ unixfs.FSCursorXattrs = (*FSCursorOps)(nil)
