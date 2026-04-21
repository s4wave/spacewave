package unixfs_block

import (
	"context"
	"io/fs"
	"slices"
	"sort"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/bits-and-blooms/bitset"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// FSTree is a handle to a filesystem tree.
// The handle can be used to manipulate the tree.
// A FSTree handle can be located at any position in the tree.
type FSTree struct {
	// ctx is the context to use for ops
	ctx context.Context
	// node is the node located at fstree root
	node *FSNode
	// bcs is a block cursor located at node
	bcs *block.Cursor
}

// NewFSTree creates a handle with a root block cursor.
//
// Ntype can be set to 0 (Unknown) to allow any.
func NewFSTree(ctx context.Context, bcs *block.Cursor, ntype NodeType) (*FSTree, error) {
	var err error
	t := &FSTree{ctx: ctx, bcs: bcs}
	t.node, err = FetchCheckFSNode(ctx, bcs, ntype)
	if err != nil {
		return nil, err
	}
	if t.node == nil {
		return nil, block.ErrNotFound
	}
	return t, nil
}

// newTxFSTree constructs a new fstree with a node object.
func newTxFSTree(ctx context.Context, bcs *block.Cursor, node *FSNode) *FSTree {
	return &FSTree{
		ctx:  ctx,
		node: node,
		bcs:  bcs,
	}
}

// GetCursor returns the cursor at Node.
func (f *FSTree) GetCursor() *block.Cursor {
	return f.bcs
}

// GetCursorRef returns the reference to the node.
func (f *FSTree) GetCursorRef() *block.BlockRef {
	return f.bcs.GetRef()
}

// GetFSNode returns the node object at f.
func (f *FSTree) GetFSNode() *FSNode {
	return f.node
}

// GetPermissions returns the permissions bits of the file mode.
func (f *FSTree) GetPermissions() (fs.FileMode, error) {
	return fs.FileMode(f.GetFSNode().GetPermissions()) & fs.ModePerm, nil
}

// SetPermissions sets the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSTree) SetPermissions(perm fs.FileMode) error {
	nperm := uint32(perm & fs.ModePerm)
	if f.node.Permissions != nperm {
		f.node.Permissions = nperm
		f.bcs.MarkDirty()
	}
	return nil
}

// SetModTimestamp changes the modification timestamp for the node.
func (f *FSTree) SetModTimestamp(ts *timestamppb.Timestamp) error {
	if !f.node.ModTime.EqualVT(ts) {
		f.node.ModTime = ts
		f.bcs.MarkDirty()
	}
	return nil
}

// BuildFileHandle builds a file handle for the node.
func (f *FSTree) BuildFileHandle(ctx context.Context) (*file.Handle, error) {
	if f.node.GetNodeType() != NodeType_NodeType_FILE {
		return nil, unixfs_errors.ErrNotFile
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
	permissions fs.FileMode,
	ts *timestamppb.Timestamp,
) (*FSTree, error) {
	if len(name) == 0 {
		return nil, unixfs_errors.ErrEmptyPath
	}

	ftree, dirent, err := f.LookupFollowDirent(name)
	if err != nil {
		// checks if f is a directory
		return nil, err
	}
	if dirent != nil {
		return ftree, unixfs_errors.ErrExist
	}

	// fetch+check the reference first
	initRefEmpty := initRef.GetEmpty()
	if !initRefEmpty {
		checkCs := f.bcs.DetachTransaction()
		checkCs.SetRefAtCursor(initRef, true)
		_, err := FetchCheckFSNode(f.ctx, checkCs, nodeType)
		if err != nil {
			return nil, err
		}
	}

	// create new entry
	dirent = &Dirent{
		Name:     name,
		NodeType: nodeType,
		NodeRef:  initRef,
	}

	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dcs := dslice.AppendDirent(dirent)
	dslice.SortDirents()

	var dnode *FSNode
	var dnodeCs *block.Cursor
	if !initRefEmpty {
		// follow the given ref to the node
		dnode, dnodeCs, err = dirent.FollowNodeRef(f.ctx, dcs)
		if err != nil {
			return nil, err
		}
	}

	// if the node is empty, create it.
	if dnode == nil {
		dnode = NewFSNode(nodeType, permissions, ts)
		dnodeCs = dcs.FollowRef(2, nil)
		dnodeCs.SetBlock(dnode, true)
	}

	return newTxFSTree(f.ctx, dnodeCs, dnode), nil
}

// Symlink creates a symbolic link from a location to a path.
// f must be a directory.
// returns a cursor to the new child node
// if checkExist, checks if name exists, returns ErrExist if so
func (f *FSTree) Symlink(
	checkExist bool,
	name string,
	lnk *FSSymlink,
	ts *timestamppb.Timestamp,
) (*FSTree, error) {
	if len(name) == 0 {
		return nil, unixfs_errors.ErrEmptyPath
	}

	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, direntIdx := dslice.LookupDirent(name)

	var dcs *block.Cursor
	if dirent != nil {
		if checkExist {
			return nil, unixfs_errors.ErrExist
		}

		// clear old dirent refs
		dcs := dslice.bcs.FollowSubBlock(uint32(direntIdx)) //nolint:gosec
		dirent.NodeRef = nil
		dcs.ClearAllRefs()

		// update dirent type
		dirent.NodeType = NodeType_NodeType_SYMLINK
	} else {
		// create new entry
		dirent = &Dirent{
			Name:     name,
			NodeType: NodeType_NodeType_SYMLINK,
		}
		dcs = dslice.AppendDirent(dirent)
		dslice.SortDirents()
	}

	dnode := NewFSNode(NodeType_NodeType_SYMLINK, DefaultPermissions(NodeType_NodeType_SYMLINK), ts)
	dnode.Symlink = lnk

	dnodeCs := dcs.FollowRef(2, nil)
	dnodeCs.SetBlock(dnode, true)

	return newTxFSTree(f.ctx, dnodeCs, dnode), nil
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
		idx: -1,
	}, nil
}

// Lookup returns a directory entry by name.
// Returns nil if not found.
func (f *FSTree) Lookup(name string) (*Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, unixfs_errors.ErrNotDirectory
	}
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, _ := ds.LookupDirent(name)
	if dirent == nil {
		return nil, nil
	}
	return dirent, dirent.Validate()
}

// LookupFollowDirent looks up and follows a directory entry by name.
// Returns nil if not found.
func (f *FSTree) LookupFollowDirent(name string) (*FSTree, *Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, nil, unixfs_errors.ErrNotDirectory
	}
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, didx := ds.LookupDirent(name)
	if dirent == nil {
		return nil, nil, nil
	}
	nfs, dirent, err := ds.FollowDirent(f.ctx, didx)
	if err != nil {
		return nil, nil, err
	}
	return nfs, dirent, nil
}

// LookupFollowDirentAsCursor looks up and follows a directory entry by name.
// Returns nil if not found.
func (f *FSTree) LookupFollowDirentAsCursor(name string) (*block.Cursor, *Dirent, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, nil, unixfs_errors.ErrNotDirectory
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
// Skip list value: found index + 1. 0 = not found.
func (f *FSTree) PreMkdir(dirs []string) (*bitset.BitSet, []int, error) {
	nodeDirs := f.node.GetDirectoryEntry()
	// target index must be at or greater than prev
	var startIdx int
	var skipBitset bitset.BitSet
	indexes := make([]int, len(dirs))
	for i := range dirs {
		if err := ValidateDirentName(dirs[i]); err != nil {
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
				return nil, indexes, unixfs_errors.ErrExist
			}
		}
		if match || (i != 0 && dirs[i] == dirs[i-1]) {
			// dir exists or is a dupe of previous entry
			skipBitset.Set(uint(i + 1)) //nolint:gosec
			continue
		}
		skipBitset.Set(0)
	}

	return &skipBitset, indexes, nil
}

// Mkdir creates one or more directories.
// May return ErrExist if any of dirs exist as a file.
func (f *FSTree) Mkdir(permissions fs.FileMode, ts *timestamppb.Timestamp, dirs ...string) (map[string]*FSTree, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, errors.New("inode is not a directory")
	}
	outputCursors := make(map[string]*FSTree, len(dirs))
	if len(dirs) == 0 {
		return outputCursors, nil
	}

	// all dirs are stored in one node, so we can do this:
	slices.Sort(dirs)
	skipBitset, skipIndexes, err := f.PreMkdir(dirs)
	if err != nil {
		return nil, err
	}

	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	for i, didx := range skipIndexes {
		// note: didx is idx + 1
		if didx != 0 {
			dirName := dirs[i]
			outputCursors[dirName], _, err = dslice.FollowDirent(f.ctx, didx-1)
			if err != nil {
				return nil, err
			}
		}
	}
	if !skipBitset.Test(0) {
		// nothing to create
		return outputCursors, nil
	}

	for i := range dirs {
		if skipBitset.Test(uint(i + 1)) { //nolint:gosec
			// already created
			continue
		}
		dirent := &Dirent{
			Name:     dirs[i],
			NodeType: NodeType_NodeType_DIRECTORY,
		}

		dcs := dslice.AppendDirent(dirent)
		dnode := NewFSNode(dirent.GetNodeType(), permissions, ts)
		dnodeCs := dcs.FollowRef(2, nil)
		dnodeCs.SetBlock(dnode, true)

		outputCursors[dirs[i]] = newTxFSTree(f.ctx, dnodeCs, dnode)
	}
	dslice.SortDirents()

	// update mod timestamp for parent node
	if ts != nil {
		f.node.ModTime = ts
	}

	return outputCursors, nil
}

// Remove removes one or more dirents from f.
// f must be a directory.
// returns if any existed.
func (f *FSTree) Remove(
	names []string,
	ts *timestamppb.Timestamp,
) (bool, error) {
	if f.GetFSNode().GetNodeType() != NodeType_NodeType_DIRECTORY {
		return false, unixfs_errors.ErrNotDirectory
	}
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	var namesSorted []string
	if len(names) <= 1 || slices.IsSorted(names) {
		namesSorted = names
	} else {
		namesSorted = make([]string, len(names))
		copy(namesSorted, names)
		sort.Strings(namesSorted)
	}
	any, err := dslice.RemoveDirents(namesSorted)
	if any && ts != nil {
		// update timestamp
		f.node.ModTime = ts
	}
	return any, err
}

// FollowDirent follows a dirent with a parent cursor.
func (f *FSTree) FollowDirent(didx int) (*FSTree, *Dirent, error) {
	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	return ds.FollowDirent(f.ctx, didx)
}

// SetDirent creates or overrides a directory pointing to the node.
func (f *FSTree) SetDirent(name string, nodeType NodeType, bcs *block.Cursor) error {
	if err := ValidateDirentName(name); err != nil {
		return err
	}

	ds := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, idx := ds.LookupDirent(name)
	var direntCs *block.Cursor
	if dirent != nil {
		dirent.NodeRef = bcs.GetRef().Clone()
		dirent.NodeType = nodeType
		direntCs = ds.bcs.FollowSubBlock(uint32(idx)) //nolint:gosec
	} else {
		direntCs = ds.AppendDirent(&Dirent{
			Name:     name,
			NodeType: nodeType,
			NodeRef:  bcs.GetRef().Clone(),
		})
		ds.SortDirents()
	}

	direntCs.SetRef(2, bcs)
	direntCs.MarkDirty()
	return nil
}
