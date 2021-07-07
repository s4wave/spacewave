package unixfs_block

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
)

// DirStream is a directory stream.
type DirStream struct {
	// ft is the fstree handle
	ft *FSTree
	// dirscs is the dirent slice cursor
	dirscs *block.Cursor
	// dirs is the dirent slice
	dirs DirentSlice
	// idx is the current dirent index
	idx int
}

// HasNext indicates if there are further entries. HasNext
// might be called on already closed streams.
func (d *DirStream) HasNext() bool {
	return d.idx+1 < d.dirs.Len()
}

// Next advances to the next entry.
// Returns EOF if there are no more.
func (d *DirStream) Next(ctx context.Context) error {
	if !d.HasNext() {
		return io.EOF
	}
	d.idx += 1
	return nil
}

// GetEntry returns the entry at the position.
func (d *DirStream) GetEntry() *Dirent {
	return d.dirs.GetDirentAtIndex(d.idx)
}

// FollowEntry returns a new handle at the entry position.
func (d *DirStream) FollowEntry() (*FSTree, *Dirent, error) {
	return d.ft.FollowDirent(d.idx)
}
