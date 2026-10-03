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
	xattrs := f.fsTree.GetFSNode().GetXattrs()
	if len(xattrs) == 0 {
		return nil, nil
	}

	// Convert filesystem node attributes to cursor attribute records.
	result := make([]unixfs.FSXattr, len(xattrs))
	for i, xa := range xattrs {
		result[i] = unixfs.FSXattr{
			Name:  xa.GetName(),
			Value: xa.GetValue(),
		}
	}
	return result, nil
}

// _ is a type assertion
var _ unixfs.FSCursorXattrs = (*FSCursorOps)(nil)
