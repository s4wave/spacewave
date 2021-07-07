package unixfs_block

import (
	"context"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/bits-and-blooms/bitset"
	"github.com/pkg/errors"
)

var (
	// ErrNotFound is returned when a block reference was not found.
	ErrNotFound = errors.New("fstree block not found")
)

// FSTree is a handle to a filesystem tree.
// The handle can be used to manipulate the tree.
// A FSTree handle can be located at any position in the tree.
// Changes are pushed to notify callbacks.
type FSTree struct {
	// node is the node located at fstree root
	node *FSNode
	// bcs is a block cursor located at node
	bcs *block.Cursor
}

// NewFSTree creates a handle with an optional root object cursor pointing to
// the tree. The cursor ref can be empty to indicate a new node.
//
// If the root ref is set, and the Fetch() returns Block not found, returns ErrNotFound.
func NewFSTree(bcs *block.Cursor, ntype NodeType) (*FSTree, error) {
	var err error
	t := &FSTree{bcs: bcs}
	t.node, err = fetchNode(bcs, ntype)
	if err != nil {
		return nil, err
	}
	if t.node == nil {
		return nil, ErrNotFound
	}
	return t, nil
}

// newTxFSTree constructs a new transaction-based fstree.
func newTxFSTree(bcs *block.Cursor, node *FSNode) *FSTree {
	// btx = nil
	return &FSTree{
		node: node,
		bcs:  bcs,
	}
}

// GetCursor returns the cursor at Node.
func (f *FSTree) GetCursor() *block.Cursor {
	return f.bcs
}

// GetCursorRef returns the reference to the node.
func (t *FSTree) GetCursorRef() *block.BlockRef {
	return t.bcs.GetRef()
}

// GetFSNode returns the node object at f.
func (f *FSTree) GetFSNode() *FSNode {
	return f.node
}

// BuildFileHandle builds a file handle for the node.
func (f *FSTree) BuildFileHandle(ctx context.Context) (*file.Handle, error) {
	if f.node.GetNodeType() != NodeType_NodeType_FILE {
		return nil, ErrNotFile
	}
	fileHandleCs := f.bcs.FollowSubBlock(4)
	return file.NewHandle(ctx, fileHandleCs, f.node.GetFile()), nil
}

// Mknod creates a node as a dirent of fstree.
// f must be a directory.
// returns a cursor to the new child node
// initRef can be nil
// checks if name exists, returns ErrExist if so
// may return the existing child and ErrExist
// slower than Mkdir for creating many directories at once
func (f *FSTree) Mknod(
	name string,
	nodeType NodeType,
	initRef *block.BlockRef,
) (*FSTree, error) {
	bcs, dirent, err := f.LookupFollowDirent(name)
	if err != nil {
		// checks if f is a directory
		return nil, err
	}
	if dirent != nil {
		return bcs, ErrExist
	}

	// create new entry
	dirent = &Dirent{
		Name:     name,
		NodeType: nodeType,
		NodeRef:  initRef,
	}
	// TODO dirent.Validate() ?
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dcs := dslice.AppendDirent(dirent)
	// move dcs to the node dirent points to
	dcs = dcs.FollowRef(2, dirent.NodeRef)
	dnode, err := fetchNode(
		dcs,
		nodeType,
	)
	if err != nil {
		return nil, err
	}
	dslice.SortDirents()
	if dnode == nil {
		return nil, errors.Errorf(
			"inode reference not found: %s",
			dcs.GetRef().MarshalString(),
		)
	}
	return newTxFSTree(dcs, dnode), nil
}

// Readdir returns a stream of directory entries.
// Returns nil if there are no directory entries.
func (f *FSTree) Readdir() (*DirStream, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, errors.New("inode is not a directory")
	}
	if len(f.node.GetDirectoryEntry()) == 0 {
		return nil, nil
	}
	return &DirStream{
		ft:     f,
		dirscs: f.bcs.FollowSubBlock(5),
		dirs: DirentSlice{
			dirents: &f.node.DirectoryEntry,
		},
		idx: 0,
	}, nil
}

// Lookup returns a directory entry by name.
// Returns nil if not found.
func (f *FSTree) Lookup(name string) (*Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, ErrNotDirectory
	}
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, _ := ds.LookupDirent(name)
	return dirent, nil
}

// LookupFollowDirent looks up and follows a directory entry by name.
// Returns nil if not found.
func (f *FSTree) LookupFollowDirent(name string) (*FSTree, *Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, nil, ErrNotDirectory
	}
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, didx := ds.LookupDirent(name)
	if dirent == nil {
		return nil, nil, nil
	}
	nfs, dirent, err := ds.FollowDirent(didx)
	if err != nil {
		return nil, nil, err
	}
	return nfs, dirent, nil
}

// LookupFollowDirentAsCursor looks up and follows a directory entry by name.
// Returns nil if not found.
func (f *FSTree) LookupFollowDirentAsCursor(name string) (*block.Cursor, *Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, nil, ErrNotDirectory
	}
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, didx := ds.LookupDirent(name)
	if dirent == nil {
		return nil, nil, nil
	}
	return ds.FollowDirentAsCursor(didx)
}

// PreMkdir checks directories for existence and returns a skip list.
// Dirs must be pre-sorted.
// Skip list is found index + 1. 0 = not found.
func (f *FSTree) PreMkdir(dirs []string) (*bitset.BitSet, []int, error) {
	nodeDirs := f.node.GetDirectoryEntry()
	// target index must be at or greater than prev
	var startIdx int
	var skipBitset bitset.BitSet
	indexes := make([]int, len(dirs))
	for i := 0; i < len(dirs); i++ {
		if err := ValidateDirectoryName(dirs[i]); err != nil {
			return nil, indexes, err
		}

		var didx int
		var match bool
		if startIdx < len(nodeDirs) {
			subslice := nodeDirs[startIdx:]
			ds := NewDirentSlice(&subslice, f.bcs)
			didx, match = ds.SearchDirents(dirs[i])
			didx += startIdx // offset
			startIdx = didx
			// note: even if not found, didx = insertion location of dir
			// since dirs is sorted, this means we can keep searching from that pt
		}
		if match {
			indexes[i] = didx + 1
			if nodeDirs[didx].GetNodeType() != NodeType_NodeType_DIRECTORY {
				return nil, indexes, ErrExist
			}
		}
		if match || (i != 0 && dirs[i] == dirs[i-1]) {
			// dir exists or is a dupe of previous entry
			skipBitset.Set(uint(i + 1))
			continue
		}
		skipBitset.Set(0)
	}
	return &skipBitset, indexes, nil
}

// Mkdir creates one or more directories.
// May return ErrExist if any of dirs exist as a file.
func (f *FSTree) Mkdir(dirs ...string) (map[string]*FSTree, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, errors.New("inode is not a directory")
	}
	outputCursors := make(map[string]*FSTree, len(dirs))
	if len(dirs) == 0 {
		return outputCursors, nil
	}

	// all dirs are stored in one node, so we can do this:
	sort.Strings(dirs)
	skipBitset, skipIndexes, err := f.PreMkdir(dirs)
	if err != nil {
		return nil, err
	}

	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	for i, didx := range skipIndexes {
		if didx != 0 {
			dirName := dirs[i]
			outputCursors[dirName], _, err = dslice.FollowDirent(didx - 1)
			if err != nil {
				return nil, err
			}
		}
	}
	if !skipBitset.Test(0) {
		// nothing to create
		return outputCursors, nil
	}

	for i := 0; i < len(dirs); i++ {
		if skipBitset.Test(uint(i + 1)) {
			// already created
			continue
		}
		dirent := &Dirent{
			Name:     dirs[i],
			NodeType: NodeType_NodeType_DIRECTORY,
			NodeRef:  &block.BlockRef{},
		}
		// TODO dirent.Validate() ?
		dcs := dslice.AppendDirent(dirent)
		// move dcs to the node dirent points to
		dcs = dcs.FollowRef(2, dirent.NodeRef)
		dnode, err := fetchNode(
			dcs,
			NodeType_NodeType_DIRECTORY,
		)
		if err != nil {
			return nil, err
		}
		if dnode == nil {
			return nil, errors.Errorf(
				"inode reference not found: %s",
				dcs.GetRef().MarshalString(),
			)
		}
		outputCursors[dirs[i]] = newTxFSTree(dcs, dnode)
	}
	dslice.SortDirents()

	return outputCursors, nil
}

// Remove removes one or more dirents from f.
// f must be a directory.
// returns if any existed.
func (f *FSTree) Remove(
	names []string,
) (bool, error) {
	if f.GetFSNode().GetNodeType() != NodeType_NodeType_DIRECTORY {
		return false, ErrNotDirectory
	}
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	var namesSorted []string
	if len(names) <= 1 || sort.StringsAreSorted(names) {
		namesSorted = names
	} else {
		namesSorted = make([]string, len(names))
		copy(namesSorted, names)
		sort.Strings(namesSorted)
	}
	any, err := dslice.RemoveDirents(namesSorted)
	if any && err == nil {
		dslice.SortDirents()
	}
	return any, err
}

// fetchNode fetches or creates a node at bcs
// rn is nil if the reference was not found.
func fetchNode(bcs *block.Cursor, ntype NodeType) (
	rn *FSNode,
	err error,
) {
	rni, _ := bcs.GetBlock()
	if rni != nil {
		rn = rni.(*FSNode)
		return
	}
	if !bcs.GetRef().GetEmpty() {
		bi, biErr := bcs.Unmarshal(NewNodeBlock)
		if biErr != nil {
			return nil, biErr
		}
		rn, _ = bi.(*FSNode)
		// rn == nil -> not found
		if rn != nil {
			if rn.GetNodeType() != ntype {
				err = errors.Errorf(
					"expected node type %v but got %v",
					ntype.String(),
					rn.GetNodeType().String(),
				)
			}
		}
	} else {
		rn = &FSNode{
			NodeType: ntype,
		}
		bcs.SetBlock(rn, true)
	}
	return
}

// FollowDirent follows a dirent with a parent cursor.
func (f *FSTree) FollowDirent(didx int) (*FSTree, *Dirent, error) {
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	return ds.FollowDirent(didx)
}
