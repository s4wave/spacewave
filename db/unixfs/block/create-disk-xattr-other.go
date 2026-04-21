//go:build windows || js

package unixfs_block

// readFileXattrs is a no-op on platforms without xattr support.
func readFileXattrs(path string) ([]*FSXattr, error) {
	return nil, nil
}
