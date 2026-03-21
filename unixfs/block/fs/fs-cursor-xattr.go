package unixfs_block_fs

import (
	"context"

	"github.com/aperturerobotics/hydra/unixfs"
)

// GetXattrs returns the extended attributes for this node.
func (f *FSCursorOps) GetXattrs(ctx context.Context) ([]unixfs.FSXattr, error) {
	if f.CheckReleased() {
		return nil, nil
	}
	xattrs := f.fsTree.GetFSNode().GetXattrs()
	if len(xattrs) == 0 {
		return nil, nil
	}
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
