package unixfs_block

import (
	"context"
	"io"
	"io/fs"
	"math"
	"slices"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// FSTree is a handle to a filesystem tree.
// The handle can be used to manipulate the tree.
// A FSTree handle can be located at any position in the tree.
type FSTree struct {
	// ctx is the context to use for ops.
	ctx context.Context
	// node is the node located at fstree root.
	node *FSNode
	// bcs is a block cursor located at node.
	bcs *block.Cursor
}

// NewFSTree creates a handle with a root block cursor.
//
// Ntype can be set to 0 (Unknown) to allow any node type.
func NewFSTree(ctx context.Context, bcs *block.Cursor, ntype NodeType) (*FSTree, error) {
	// Load the filesystem node at the root cursor and require it to exist.
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

// GetCursorRef returns the reference to the node.
func (f *FSTree) GetCursorRef() *block.BlockRef {
	return f.bcs.GetRef()
}

// GetNodeType returns the inode type.
func (f *FSTree) GetNodeType() NodeType {
	return f.node.GetNodeType()
}

// FileSize returns the file content size, or zero for an inode without a file.
func (f *FSTree) FileSize() uint64 {
	return f.node.GetFile().GetTotalSize()
}

// Size returns the file size or the length of a symbolic link's target path.
func (f *FSTree) Size() uint64 {
	if f.node.GetNodeType().GetIsSymlink() {
		tp := f.node.GetSymlink().GetTargetPath()
		return uint64(len(unixfs.JoinPath(tp.GetNodes(), tp.GetAbsolute())))
	}
	return f.FileSize()
}

// ModTime returns the inode modification time.
func (f *FSTree) ModTime() time.Time {
	return f.node.GetModTime().AsTime()
}

// Snapshot clones the inode for comparisons after the tree's transaction ends.
func (f *FSTree) Snapshot() *FSNode {
	return f.node.CloneVT()
}

// DirectoryEntries yields directory entries in name order with their indices.
// Callers must not change the directory while enumerating it.
func (f *FSTree) DirectoryEntries(yield func(int, *Dirent) bool) {
	for i, ent := range f.node.GetDirectoryEntry() {
		if !yield(i, ent) {
			return
		}
	}
}

// Readlink returns the symbolic link target or ErrNotSymlink for another type.
func (f *FSTree) Readlink() ([]string, bool, error) {
	if f.node.GetNodeType() != NodeType_NodeType_SYMLINK {
		return nil, false, unixfs_errors.ErrNotSymlink
	}
	tp := f.node.GetSymlink().GetTargetPath()
	return tp.GetNodes(), tp.GetAbsolute(), nil
}

// GetXattrs converts the inode's extended attributes to filesystem records.
func (f *FSTree) GetXattrs() []unixfs.FSXattr {
	// Keep an empty attribute list nil, as cursor attribute reads require.
	xattrs := f.node.GetXattrs()
	if len(xattrs) == 0 {
		return nil
	}

	// Copy attribute records while retaining their existing value slices.
	result := make([]unixfs.FSXattr, len(xattrs))
	for i, xa := range xattrs {
		result[i] = unixfs.FSXattr{Name: xa.GetName(), Value: xa.GetValue()}
	}
	return result
}

// GetXattrValue returns the named extended attribute, or nil when absent.
func (f *FSTree) GetXattrValue(name string) []byte {
	return f.node.GetXattrValue(name)
}

// SetXattr sets or replaces the named extended attribute and marks the inode
// dirty.
func (f *FSTree) SetXattr(name string, value []byte) {
	f.node.SetXattr(name, value)
	f.bcs.MarkDirty()
}

// RemoveXattr removes the named extended attribute, marking the inode dirty
// when it was present. It reports whether the attribute was present.
func (f *FSTree) RemoveXattr(name string) bool {
	if !f.node.RemoveXattr(name) {
		return false
	}
	f.bcs.MarkDirty()
	return true
}

// ImportXattrs adds attributes to a newly created inode already marked dirty.
func (f *FSTree) ImportXattrs(xattrs []*FSXattr) {
	for _, xa := range xattrs {
		f.node.SetXattr(xa.GetName(), xa.GetValue())
	}
}

// InitFileWithReader builds the inode's file content from a reader.
// The caller must have created and marked the inode dirty before calling.
func (f *FSTree) InitFileWithReader(ctx context.Context, rdr io.Reader, opts *blob.BuildBlobOpts) error {
	var err error
	f.node.File, err = file.BuildFileWithReader(ctx, f.bcs.FollowSubBlock(4), rdr, opts)
	return err
}

// InitFileWithBytes builds the inode's file content from bytes.
// The caller must have created and marked the inode dirty before calling.
func (f *FSTree) InitFileWithBytes(ctx context.Context, data []byte) error {
	var err error
	f.node.File, err = file.BuildFileWithBytes(ctx, f.bcs.FollowSubBlock(4), data, nil)
	return err
}

// copyTo attaches a detached copy of this inode to the destination directory.
func (f *FSTree) copyTo(dest *FSTree, name string) error {
	nodeType := f.node.GetNodeType()
	bcs := f.bcs.DetachRecursive(true, true, true)
	return dest.SetDirent(name, nodeType, bcs)
}

// GetPermissions returns the permissions bits of the file mode.
func (f *FSTree) GetPermissions() (fs.FileMode, error) {
	return fs.FileMode(f.node.GetPermissions()) & fs.ModePerm, nil
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
// f must be a directory. Returns a cursor to the new child node. initRef can
// be nil. Returns ErrExist, possibly with the existing child,
// when name exists. Slower than Mkdir for creating many directories at once.
func (f *FSTree) Mknod(
	name string,
	nodeType NodeType,
	initRef *block.BlockRef,
	permissions fs.FileMode,
	ts *timestamppb.Timestamp,
) (*FSTree, error) {
	// Require a directory entry name before creating an inode.
	if len(name) == 0 {
		return nil, unixfs_errors.ErrEmptyPath
	}

	// Check the parent directory for an existing entry with this name.
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

	// Attach the new entry to the sorted directory slice.
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dcs := dslice.AppendDirent(dirent)
	dslice.SortDirents()

	// Load the initial inode when the entry references an existing block.
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
// f must be a directory. Returns a cursor to the new child node. If
// checkExist, returns ErrExist when name exists.
func (f *FSTree) Symlink(
	checkExist bool,
	name string,
	lnk *FSSymlink,
	ts *timestamppb.Timestamp,
) (*FSTree, error) {
	// Require a directory entry name before creating a symbolic link.
	if len(name) == 0 {
		return nil, unixfs_errors.ErrEmptyPath
	}

	// Find the directory entry that will hold the symbolic link.
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	dirent, direntIdx := dslice.LookupDirent(name)

	// Replace the existing entry or append a new symbolic link entry.
	var dcs *block.Cursor
	if dirent != nil {
		if checkExist {
			return nil, unixfs_errors.ErrExist
		}

		// Clear the replaced entry's node reference.
		dcs = dslice.bcs.FollowSubBlock(uint32(direntIdx)) //nolint:gosec
		dirent.NodeRef = nil
		dcs.ClearAllRefs()

		// Mark the replaced entry as a symbolic link.
		dirent.NodeType = NodeType_NodeType_SYMLINK
	} else {
		// Append a new symbolic link entry in sorted order.
		dirent = &Dirent{
			Name:     name,
			NodeType: NodeType_NodeType_SYMLINK,
		}
		dcs = dslice.AppendDirent(dirent)
		dslice.SortDirents()
	}

	// Construct the symbolic link inode with its target path.
	dnode := NewFSNode(NodeType_NodeType_SYMLINK, DefaultPermissions(NodeType_NodeType_SYMLINK), ts)
	dnode.Symlink = lnk

	// Attach the symbolic link inode to the directory entry cursor.
	dnodeCs := dcs.FollowRef(2, nil)
	dnodeCs.SetBlock(dnode, true)

	return newTxFSTree(f.ctx, dnodeCs, dnode), nil
}

// Readdir returns a stream of directory entries.
// Returns nil if there are no directory entries.
func (f *FSTree) Readdir() (*DirStream, error) {
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, unixfs_errors.ErrNotDirectory
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
	// Require a directory before looking up an entry by name.
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
	// Require a directory before following an entry to its inode.
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
	// Require a directory before following an entry to its block cursor.
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

// Mkdir creates one or more directories.
// May return ErrExist if any of dirs exist as a file.
func (f *FSTree) Mkdir(permissions fs.FileMode, ts *timestamppb.Timestamp, dirs ...string) (map[string]*FSTree, error) {
	// Require a directory and prepare cursors for the requested children.
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return nil, unixfs_errors.ErrNotDirectory
	}
	outputCursors := make(map[string]*FSTree, len(dirs))
	if len(dirs) == 0 {
		return outputCursors, nil
	}

	// Sort dirs together because all dirs are stored in one node.
	slices.Sort(dirs)
	nodeDirs := f.node.GetDirectoryEntry()
	skipDirs := make([]bool, len(dirs))
	skipIndexes := make([]int, len(dirs))
	var hasCreate bool

	// Identify existing children and duplicate names before creating directories.
	var startIdx int
	for i := range dirs {
		if err := ValidateDirentName(dirs[i]); err != nil {
			return nil, err
		}

		var didx int
		var match bool
		if startIdx < len(nodeDirs) {
			subslice := nodeDirs[startIdx:]
			ds := NewDirentSlice(&subslice, f.bcs)
			didx, match = ds.SearchDirents(dirs[i])
			didx += startIdx
			startIdx = didx
		}
		if match {
			skipIndexes[i] = didx + 1
			if nodeDirs[didx].GetNodeType() != NodeType_NodeType_DIRECTORY {
				return nil, unixfs_errors.ErrExist
			}
		}
		// #nosec G602 -- dirs[i-1] is guarded by the i != 0 short-circuit in this condition.
		if match || (i != 0 && dirs[i] == dirs[i-1]) {
			skipDirs[i] = true
			continue
		}
		hasCreate = true
	}

	// Open cursors for the requested directories that already exist.
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	var err error
	for i, didx := range skipIndexes {
		// note: didx is idx + 1
		if didx != 0 {
			dirName := dirs[i] //nolint:gosec
			outputCursors[dirName], _, err = dslice.FollowDirent(f.ctx, didx-1)
			if err != nil {
				return nil, err
			}
		}
	}
	if !hasCreate {
		// nothing to create
		return outputCursors, nil
	}

	// Create inode blocks and directory entries for the missing children.
	for i := range dirs {
		if skipDirs[i] {
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
	// Require a directory before removing its entries.
	if f.node.GetNodeType() != NodeType_NodeType_DIRECTORY {
		return false, unixfs_errors.ErrNotDirectory
	}
	dslice := NewDirentSlice(&f.node.DirectoryEntry, f.bcs)
	var namesSorted []string
	if len(names) <= 1 || slices.IsSorted(names) {
		namesSorted = names
	} else {
		namesSorted = make([]string, len(names))
		copy(namesSorted, names)
		slices.Sort(namesSorted)
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
	// Validate the directory entry name before attaching an inode.
	if err := ValidateDirentName(name); err != nil {
		return err
	}

	// Replace or append the directory entry with the supplied inode reference.
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

	// Attach the inode cursor and mark the directory entry dirty.
	direntCs.SetRef(2, bcs)
	direntCs.MarkDirty()
	return nil
}

// WriteAt writes a byte range in this file and publishes its modification time.
func (f *FSTree) WriteAt(ctx context.Context, blobOpts *blob.BuildBlobOpts, offset int64, writeLen int64, dataRdr io.Reader, ts *timestamppb.Timestamp) error {
	// Open the destination file handle for the write.
	fh, err := f.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	// Write the supplied bytes into the destination file range.
	writer := file.NewWriter(fh, nil, blobOpts)
	err = writer.WriteFrom(uint64(offset), writeLen, dataRdr) //nolint:gosec
	if err != nil {
		return err
	}

	// Set a placeholder timestamp if ts is nil.
	ts = FillPlaceholderTimestamp(ts)
	f.node.ModTime = ts

	// Publish the changed file node to the block cursor.
	f.bcs.SetBlock(f.node, true)
	return nil
}

// WriteBlob writes a referenced blob into this file, merging sequential writes when allowed.
func (f *FSTree) WriteBlob(ctx context.Context, offset uint64, blobRef *block.BlockRef, fullValidate bool, forceUseBlob bool, ts *timestamppb.Timestamp) error {
	// Load the referenced blob in a detached transaction.
	blobCs := f.bcs.DetachTransaction()
	blobCs.SetRefAtCursor(blobRef, true)
	blk, err := blobCs.Unmarshal(ctx, blob.NewBlobBlock)
	if err != nil {
		return err
	}
	bl, ok := blk.(*blob.Blob)
	if !ok {
		return block.ErrUnexpectedType
	}

	// Require blob contents before writing a file range.
	totalSize := bl.GetTotalSize()
	if totalSize == 0 {
		return unixfs_errors.ErrEmptyBlob
	}

	// Open the destination file handle for the blob write.
	fh, err := f.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	// Prepare the file writer for merging or referencing the blob.
	writer := file.NewWriter(fh, nil, nil)

	// optimization: sequential writes: if the blob starts at the end of
	// the current file, use the WriteBytes call instead to re-chunk the
	// data and merge it into the previous Range in the file.
	if !forceUseBlob && f.FileSize() == offset {
		br, err := blob.NewReader(ctx, blobCs)
		if err != nil {
			return err
		}
		err = writer.WriteFrom(offset, int64(totalSize), br) //nolint:gosec
		_ = br.Close()
		return err
	}

	// Validate the full blob if necessary.
	if fullValidate {
		if err := bl.ValidateFull(ctx, blobCs); err != nil {
			return err
		}
	}

	// otherwise, append the blob to the file & sort (slower)
	err = writer.WriteBlob(offset, totalSize, blobRef)
	if err != nil {
		return err
	}

	// update timestamp
	// Set a placeholder timestamp if ts is nil.
	ts = FillPlaceholderTimestamp(ts)
	f.node.ModTime = ts

	// Publish the changed file node to the block cursor.
	f.bcs.SetBlock(f.node, true)
	return nil
}

// TruncateFile resizes this file and publishes its modification time.
func (f *FSTree) TruncateFile(ctx context.Context, nsize int64, ts *timestamppb.Timestamp) error {
	// Open the file handle for the size change.
	fh, err := f.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	// Keep a file with the requested size unchanged.
	if fh.Size() == uint64(nsize) { //nolint:gosec
		// no-op
		return nil
	}

	// Resize the file through its block-backed writer.
	writer := file.NewWriter(fh, nil, nil)
	err = writer.Truncate(uint64(nsize)) //nolint:gosec
	if err != nil {
		return err
	}

	// update timestamp
	f.node.ModTime = ts

	// Publish the resized file node to the block cursor.
	f.bcs.SetBlock(f.node, true)
	return nil
}

// MknodWithContent creates a file at the given path and writes blob content to it.
// MknodWithContent is atomic: the file is fully formed after the call.
// path must include the file name as the last element.
// blobRef is the reference to a pre-built blob containing the file data.
func (f *FSTree) MknodWithContent(
	ctx context.Context,
	path []string,
	nodeType NodeType,
	permissions fs.FileMode,
	ts *timestamppb.Timestamp,
	blobRef *block.BlockRef,
) error {
	// Require a destination path before creating a file with content.
	if len(path) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	// Fill the creation timestamp for the new file node.
	ts = FillPlaceholderTimestamp(ts)

	// Step 1: navigate to the parent directory.
	parentPath := path[:len(path)-1]
	parentNode, _, err := LookupFSTreePath(f, parentPath)
	if err != nil {
		return err
	}

	// Step 2: create the file entry.
	fileName := path[len(path)-1]
	childFtree, err := parentNode.Mknod(fileName, nodeType, nil, permissions, ts)
	if err != nil {
		return err
	}

	// Step 3: write pre-built blob content into the file.
	// forceUseBlob=true: reference the blob directly, don't re-read and re-chunk.
	// fullValidate=false: the caller built the blob, so no need to validate
	// again.
	if !blobRef.GetEmpty() {
		return WriteBlob(ctx, f, path, 0, blobRef, false, true, ts)
	}

	// Empty blob: only mark the file node dirty.
	childFtree.bcs.MarkDirty()
	return nil
}

// BuildContent builds and flushes a detached blob for atomic file creation.
func (f *FSTree) BuildContent(ctx context.Context, dataLen int64, rdr io.Reader) (*block.BlockRef, error) {
	// Build blob in a detached transaction.
	blobCs := f.bcs.DetachTransaction()
	blobCs.SetRefAtCursor(nil, true)
	_, err := blob.BuildBlob(ctx, dataLen, rdr, blobCs, nil)
	if err != nil {
		return nil, err
	}

	// Write the detached transaction to flush blocks and compute the blob ref.
	// Without the write, GetRef() returns nil because the block has not
	// been hashed.
	blobRef, _, err := blobCs.GetTransaction().Write(ctx, true)
	if err != nil {
		return nil, err
	}
	if blobRef == nil {
		blobRef = &block.BlockRef{}
	}

	return blobRef, nil
}

// MatchesFileInfo compares a file's size, modification time and permissions.
func (f *FSTree) MatchesFileInfo(info fs.FileInfo) bool {
	return f.FileSize() == uint64(info.Size()) && //nolint:gosec // a file size is not negative.
		f.ModTime().Equal(info.ModTime()) &&
		fs.FileMode(f.node.GetPermissions()) == info.Mode().Perm()
}

// OpenVersion opens an inode reference in a detached transaction.
func (f *FSTree) OpenVersion(ctx context.Context, ref *block.BlockRef) (*FSTree, error) {
	bcs := f.bcs.DetachTransaction()
	bcs.SetRefAtCursor(ref, true)
	return NewFSTree(ctx, bcs, NodeType_NodeType_UNKNOWN)
}

// OpenBlob opens detached content whose size fits the signed reader interface.
// The caller must close the returned reader.
func (f *FSTree) OpenBlob(ctx context.Context, ref *block.BlockRef) (int64, io.ReadCloser, error) {
	// Load the incoming blob independently of the filesystem transaction.
	bcs := f.bcs.DetachTransaction()
	bcs.SetRefAtCursor(ref, true)
	bl, err := blob.UnmarshalBlob(ctx, bcs)
	if err != nil {
		return 0, nil, err
	}
	if bl.GetTotalSize() > math.MaxInt64 {
		return 0, nil, errors.New("source blob size exceeds int64")
	}

	// Open a reader after validating the declared length.
	size := int64(bl.GetTotalSize()) //nolint:gosec
	rdr, err := blob.NewReader(ctx, bcs)
	return size, rdr, err
}
