package unixfs_block

import (
	"github.com/aperturerobotics/hydra/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
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
// returns false if there are no more entries.
func (d *DirStream) Next() bool {
	if !d.HasNext() {
		return false
	}
	d.idx += 1
	return true
}

// Skip skips N entries in either direction.
// Returns the new index (which may not be in bounds!).
func (d *DirStream) Skip(n int) int {
	d.idx += n
	return d.idx
}

// GetEntry returns the entry at the position.
// Note: call Next() at least once before GetEntry.
func (d *DirStream) GetEntry() *Dirent {
	if d.idx < 0 || d.idx >= d.dirs.Len() {
		return nil
	}
	return d.dirs.GetDirentAtIndex(d.idx)
}

// FollowEntry returns a new handle at the entry position.
// ensures that the next node type is as expected if expectedNodeType is not UNKNOWN.
func (d *DirStream) FollowEntry(expectedNodeType NodeType) (*FSTree, *Dirent, error) {
	if d.idx < 0 || d.idx >= d.dirs.Len() {
		return nil, nil, unixfs_errors.ErrOutOfBounds
	}
	return d.ft.FollowDirent(d.idx, expectedNodeType)
}
