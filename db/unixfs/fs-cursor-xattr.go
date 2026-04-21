package unixfs

import "context"

// FSCursorXattrs is an optional interface for FSCursorOps that supports
// extended attributes. Implementations that store xattrs (e.g. block-backed)
// should implement this interface.
type FSCursorXattrs interface {
	// GetXattrs returns the extended attributes for this node.
	// Returns nil if the node has no xattrs.
	GetXattrs(ctx context.Context) ([]FSXattr, error)
}

// FSXattr is an extended attribute name/value pair.
type FSXattr struct {
	// Name is the xattr key.
	Name string
	// Value is the xattr value bytes.
	Value []byte
}
