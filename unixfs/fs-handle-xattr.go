package unixfs

import "context"

// GetXattrs returns the extended attributes for this node.
// Returns nil if the underlying cursor does not support xattrs.
func (h *FSHandle) GetXattrs(ctx context.Context) ([]FSXattr, error) {
	var xattrs []FSXattr
	err := h.i().accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
		xattrOps, ok := ops.(FSCursorXattrs)
		if !ok {
			return nil
		}
		var err error
		xattrs, err = xattrOps.GetXattrs(ctx)
		return err
	})
	return xattrs, err
}
