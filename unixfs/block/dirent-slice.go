package unixfs_block

import (
	"errors"
	"sort"

	"github.com/aperturerobotics/hydra/block"
)

// DirentSlice implements dirent slice functions.
type DirentSlice struct {
	dirents *[]*Dirent
	// bcs may be nil
	// should be located at the dirent slice sub-block
	bcs *block.Cursor
	// writing is set to indicate we are in the pre-write hook
	writing bool
}

// NewDirentSlice builds a new DirentSlice from a slice pointer.
func NewDirentSlice(dirents *[]*Dirent, parentNodeCursor *block.Cursor) *DirentSlice {
	ds := &DirentSlice{dirents: dirents}
	if parentNodeCursor != nil {
		ds.bcs = parentNodeCursor.FollowSubBlock(5)
	}
	return ds
}

// Len is the number of elements in the collection.
func (d *DirentSlice) Len() int {
	if d.dirents == nil {
		return 0
	}
	return len(*d.dirents)
}

// Less reports whether the element with
// index i should sort before the element with index j.
// does not do bounds checks
func (d *DirentSlice) Less(i, j int) bool {
	if d.dirents == nil {
		return false
	}
	dirents := *d.dirents
	return dirents[i].GetName() < dirents[j].GetName()
}

// Swap swaps the elements with indexes i and j.
// If bcs is set on dirent slice, also swaps reference ids.
func (d *DirentSlice) Swap(i, j int) {
	if d.dirents == nil {
		return
	}
	dirents := *d.dirents

	if d.bcs != nil {
		iref := d.bcs.FollowRef(uint32(i), dirents[i].GetNodeRef())
		jref := d.bcs.FollowRef(uint32(j), dirents[j].GetNodeRef())
		// swap
		d.bcs.SetRef(uint32(i), jref)
		d.bcs.SetRef(uint32(j), iref)
	}

	// swap slice positions
	p := dirents[i]
	dirents[i] = dirents[j]
	dirents[j] = p
}

// ApplySubBlock applies a sub-block change with a field id.
func (d *DirentSlice) ApplySubBlock(id uint32, next block.SubBlock) error {
	direntSlice := *d.dirents
	dirent, ok := next.(*Dirent)
	if !ok {
		return errors.New("dirent slice sub-block must be a dirent")
	}
	if int(id) >= len(direntSlice) {
		ds := make([]*Dirent, id+1)
		copy(ds, direntSlice)
		direntSlice = ds
	}
	direntSlice[id] = dirent
	*d.dirents = direntSlice
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (d *DirentSlice) GetSubBlocks() map[uint32]block.SubBlock {
	direntSlice := *d.dirents
	if len(direntSlice) == 0 {
		return nil
	}

	m := make(map[uint32]block.SubBlock)
	for idx, dirent := range direntSlice {
		if dirent == nil {
			continue
		}
		m[uint32(idx)] = dirent
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (d *DirentSlice) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	return func(create bool) block.SubBlock {
		direntSlice := *d.dirents
		if int(id) >= len(direntSlice) {
			if !create {
				return nil
			}
			ds := make([]*Dirent, id+1)
			copy(ds, direntSlice)
			direntSlice = ds
		}
		dirent := direntSlice[id]
		if dirent == nil && create {
			dirent = &Dirent{}
		}
		return dirent
	}
}

// BlockPreWriteHook is called when writing the block.
func (d *DirentSlice) BlockPreWriteHook() error {
	b := d.bcs
	d.bcs = nil // avoid deadlock swapping on cursor
	d.SortDirents()
	d.bcs = b
	return nil
}

// SearchDirents searches a dirent slice for a name.
// If not found returns the index it should be inserted.
func (d *DirentSlice) SearchDirents(name string) (idx int, match bool) {
	if d.dirents == nil {
		return -1, false
	}
	dirents := *d.dirents
	didx := sort.Search(len(dirents), func(idx int) bool {
		return name <= dirents[idx].GetName()
	})
	if didx >= len(dirents) || didx < 0 {
		return didx, false
	}
	return didx, dirents[didx].GetName() == name
}

// SortDirents sorts a dirent slice.
func (d *DirentSlice) SortDirents() {
	sort.Sort(d)
}

// GetDirentAtIndex returns a dirent at an index.
func (d *DirentSlice) GetDirentAtIndex(i int) *Dirent {
	if d.dirents == nil {
		return nil
	}
	dirents := *d.dirents
	if i < 0 || i >= len(dirents) {
		return nil
	}
	return dirents[i]
}

// LookupDirent looks for a dirent with a given name.
// returns nil if not found.
func (d *DirentSlice) LookupDirent(name string) (*Dirent, int) {
	if d.dirents == nil {
		return nil, 0
	}
	didx, match := d.SearchDirents(name)
	if !match {
		return nil, didx
	}
	return (*d.dirents)[didx], didx
}

// FollowDirentAsCursor follows a directory entry to its node reference.
// bcs must be set on the dirent slice
// ensures that the next node type is as expected
// may return ErrOutOfBounds
func (d *DirentSlice) FollowDirentAsCursor(didx int) (*block.Cursor, *Dirent, error) {
	if d.dirents == nil || d.bcs == nil {
		return nil, nil, ErrOutOfBounds
	}
	dirents := *d.dirents
	if didx < 0 || didx >= len(dirents) {
		return nil, nil, ErrOutOfBounds
	}

	dirent := dirents[didx]
	subRef := d.bcs.FollowSubBlock(uint32(didx))
	nodeRef := subRef.FollowRef(2, dirent.GetNodeRef())
	return nodeRef, dirent, nil
}

// FollowDirent follows a directory entry to its node reference.
// bcs must be set on the dirent slice
// ensures that the next node type is as expected
// may return ErrOutOfBounds
func (d *DirentSlice) FollowDirent(didx int) (*FSTree, *Dirent, error) {
	nodeRef, dirent, err := d.FollowDirentAsCursor(didx)
	if err != nil {
		return nil, dirent, err
	}

	node, err := fetchNode(nodeRef, dirent.GetNodeType())
	if err != nil {
		return nil, dirent, err
	}
	return newTxFSTree(nodeRef, node), dirent, nil
}

// RemoveDirents removes one or more directory entries.
// names must be sorted.
// returns if any were removed.
// after removing all entries be sure to call SortDirents.
func (d *DirentSlice) RemoveDirents(names []string) (bool, error) {
	if d.dirents == nil || len(names) == 0 {
		return false, nil
	}

	dirents := *d.dirents
	nextName := names[0]
	var any bool
DirentLoop:
	for di := 0; di < len(dirents); di++ {
		dirent := dirents[di]
		direntName := dirent.GetName()
		for direntName > nextName {
			names = names[1:]
			if len(names) == 0 {
				break DirentLoop
			}
			nextName = names[0]
		}
		if direntName == nextName {
			any = true
			names = names[1:]
			// clear old reference
			if d.bcs != nil {
				d.bcs.ClearRef(uint32(di))
			}
			if di+1 < len(dirents) {
				// update block graph with pre-remove swap
				swapIdx := len(dirents) - 1
				if d.bcs != nil {
					sb := d.bcs.FollowSubBlock(uint32(swapIdx))
					d.bcs.SetRef(uint32(di), sb)
				}
				dirents[di] = dirents[swapIdx]
			}
			// remove dirent from slice
			dirents = dirents[:len(dirents)-1]
			if len(dirents) == 0 {
				dirents = nil
			}
			*d.dirents = dirents
			if d.bcs != nil {
				d.bcs.SetBlock(d, true)
			}
			if len(names) == 0 {
				break DirentLoop
			} else {
				nextName = names[0]
			}
		}
	}

	// NOTE: call SortDirents if any is true!
	return any, nil
}

// AppendDirent appends a entry to the dirent slice.
// Ensure the entry does not exist BEFORE calling this.
// After appending all directories, be sure to call SortDirents.
func (d *DirentSlice) AppendDirent(nent *Dirent) *block.Cursor {
	if d.dirents == nil {
		return nil
	}
	nextIdx := len(*d.dirents)
	*d.dirents = append(*d.dirents, nent)
	if d.bcs == nil {
		return nil
	}

	subBlk := d.bcs.FollowSubBlock(uint32(nextIdx))
	subBlk.SetBlock(nent, true)
	return subBlk
}

// _ is a type assertion
var (
	_ sort.Interface              = ((*DirentSlice)(nil))
	_ block.SubBlock              = ((*DirentSlice)(nil))
	_ block.BlockWithSubBlocks    = ((*DirentSlice)(nil))
	_ block.BlockWithPreWriteHook = ((*DirentSlice)(nil))
)
