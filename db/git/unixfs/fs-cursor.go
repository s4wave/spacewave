// Package unixfs_git implements a git-backed FSCursor.
package unixfs_git

import (
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// GitFSCursor implements unixfs.FSCursor backed by a go-git tree.
type GitFSCursor struct {
	isReleased atomic.Bool
	storer     storer.EncodedObjectStorer
	tree       *object.Tree      // current directory tree (nil for files/symlinks)
	entry      *object.TreeEntry // tree entry for this node (nil for root)
	name       string
}

// NewGitFSCursor creates a new git-backed FSCursor for a directory.
// storer is the object storer for resolving hashes to objects.
// tree is the git tree object for this directory.
// name is the directory name (empty string for root).
func NewGitFSCursor(storer storer.EncodedObjectStorer, tree *object.Tree, name string) *GitFSCursor {
	return &GitFSCursor{
		storer: storer,
		tree:   tree,
		name:   name,
	}
}

// newGitFSCursorFromEntry creates a cursor from a tree entry.
func newGitFSCursorFromEntry(s storer.EncodedObjectStorer, entry *object.TreeEntry) (*GitFSCursor, error) {
	c := &GitFSCursor{
		storer: s,
		entry:  entry,
		name:   entry.Name,
	}
	if entry.Mode == filemode.Dir || entry.Mode == filemode.Submodule {
		tree, err := object.GetTree(s, entry.Hash)
		if err != nil {
			if entry.Mode == filemode.Submodule {
				// submodule tree may not be available; treat as empty dir
				c.tree = &object.Tree{}
				return c, nil
			}
			return nil, err
		}
		c.tree = tree
	}
	return c, nil
}

// CheckReleased checks if the cursor is released.
func (c *GitFSCursor) CheckReleased() bool {
	return c.isReleased.Load()
}

// GetProxyCursor returns nil, nil (no proxy needed for immutable data).
func (c *GitFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return nil, nil
}

// AddChangeCb is a no-op (immutable tree never changes).
func (c *GitFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	// noop
}

// GetCursorOps returns the FSCursorOps for this cursor.
func (c *GitFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return newGitFSCursorOps(c), nil
}

// Release releases the cursor.
func (c *GitFSCursor) Release() {
	c.isReleased.Store(true)
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*GitFSCursor)(nil))

// GitFSCursorOps implements unixfs.FSCursorOps for git trees.
type GitFSCursorOps struct {
	isReleased atomic.Bool
	cursor     *GitFSCursor
	storer     storer.EncodedObjectStorer
	tree       *object.Tree      // for directory ops
	entry      *object.TreeEntry // for file/symlink ops
	name       string
	isDir      bool
	isSymlink  bool
}

// newGitFSCursorOps creates ops from a cursor.
func newGitFSCursorOps(c *GitFSCursor) *GitFSCursorOps {
	ops := &GitFSCursorOps{
		cursor: c,
		storer: c.storer,
		tree:   c.tree,
		entry:  c.entry,
		name:   c.name,
	}
	if c.tree != nil {
		ops.isDir = true
	} else if c.entry != nil {
		ops.isSymlink = c.entry.Mode == filemode.Symlink
	}
	return ops
}

// CheckReleased checks if the ops is released.
func (o *GitFSCursorOps) CheckReleased() bool {
	if o == nil {
		return true
	}
	return o.isReleased.Load() || o.cursor.CheckReleased()
}

// GetName returns the name of the node.
func (o *GitFSCursorOps) GetName() string {
	return o.name
}

// GetIsDirectory returns if the node is a directory.
func (o *GitFSCursorOps) GetIsDirectory() bool {
	return o.isDir
}

// GetIsFile returns if the node is a regular file.
func (o *GitFSCursorOps) GetIsFile() bool {
	return !o.isDir && !o.isSymlink
}

// GetIsSymlink returns if the node is a symlink.
func (o *GitFSCursorOps) GetIsSymlink() bool {
	return o.isSymlink
}

// GetPermissions returns the file mode permissions.
func (o *GitFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.entry == nil {
		// root directory
		return 0o755 | fs.ModeDir, nil
	}
	m, err := o.entry.Mode.ToOSFileMode()
	if err != nil {
		return 0, err
	}
	return m, nil
}

// SetPermissions returns ErrReadOnly.
func (o *GitFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// GetSize returns the size of the node in bytes.
func (o *GitFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.isDir {
		return 0, nil
	}
	if o.entry == nil {
		return 0, nil
	}
	blob, err := object.GetBlob(o.storer, o.entry.Hash)
	if err != nil {
		return 0, err
	}
	return uint64(blob.Size), nil //nolint:gosec
}

// GetModTimestamp returns the modification timestamp.
// Git trees do not have timestamps; returns zero time.
func (o *GitFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return time.Time{}, nil
}

// SetModTimestamp returns ErrReadOnly.
func (o *GitFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// ReadAt reads from a file node at the given offset.
func (o *GitFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}
	if o.entry == nil {
		return 0, unixfs_errors.ErrNotFile
	}

	blob, err := object.GetBlob(o.storer, o.entry.Hash)
	if err != nil {
		return 0, err
	}

	if offset >= blob.Size {
		return 0, io.EOF
	}

	reader, err := blob.Reader()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	// skip to offset
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, reader, offset); err != nil {
			return 0, err
		}
	}

	n, err := io.ReadFull(reader, data)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return int64(n), err
}

// GetOptimalWriteSize returns 0, ErrReadOnly.
func (o *GitFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 0, unixfs_errors.ErrReadOnly
}

// WriteAt returns ErrReadOnly.
func (o *GitFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Truncate returns ErrReadOnly.
func (o *GitFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Lookup looks up a child entry in a directory.
func (o *GitFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.isDir {
		return nil, unixfs_errors.ErrNotDirectory
	}

	entry, err := o.tree.FindEntry(name)
	if err != nil {
		return nil, unixfs_errors.ErrNotExist
	}

	cursor, err := newGitFSCursorFromEntry(o.storer, entry)
	if err != nil {
		return nil, err
	}
	return cursor, nil
}

// ReaddirAll reads all directory entries.
func (o *GitFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.isDir {
		return unixfs_errors.ErrNotDirectory
	}

	for i := int(skip); i < len(o.tree.Entries); i++ { //nolint:gosec
		entry := &o.tree.Entries[i]
		d := &gitDirent{
			name:      entry.Name,
			isDir:     entry.Mode == filemode.Dir || entry.Mode == filemode.Submodule,
			isSymlink: entry.Mode == filemode.Symlink,
		}
		d.isFile = !d.isDir && !d.isSymlink
		if err := cb(d); err != nil {
			return err
		}
	}
	return nil
}

// Mknod returns ErrReadOnly.
func (o *GitFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Symlink returns ErrReadOnly.
func (o *GitFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, tgtIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Readlink reads a symbolic link's target.
// If name is empty, reads the link at the cursor position.
func (o *GitFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}

	var entry *object.TreeEntry
	if name == "" {
		if !o.isSymlink {
			return nil, false, unixfs_errors.ErrNotSymlink
		}
		entry = o.entry
	} else {
		if !o.isDir {
			return nil, false, unixfs_errors.ErrNotDirectory
		}
		e, err := o.tree.FindEntry(name)
		if err != nil {
			return nil, false, unixfs_errors.ErrNotExist
		}
		if e.Mode != filemode.Symlink {
			return nil, false, unixfs_errors.ErrNotSymlink
		}
		entry = e
	}

	blob, err := object.GetBlob(o.storer, entry.Hash)
	if err != nil {
		return nil, false, err
	}

	reader, err := blob.Reader()
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, false, err
	}

	target := string(data)
	isAbsolute := path.IsAbs(target)
	target = strings.TrimPrefix(target, "/")
	parts := strings.Split(target, "/")
	return parts, isAbsolute, nil
}

// CopyTo returns false, nil (not implemented).
func (o *GitFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

// CopyFrom returns false, nil (not implemented).
func (o *GitFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

// MoveTo returns false, ErrReadOnly.
func (o *GitFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// MoveFrom returns false, ErrReadOnly.
func (o *GitFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// Remove returns ErrReadOnly.
func (o *GitFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// MknodWithContent returns ErrReadOnly.
func (o *GitFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*GitFSCursorOps)(nil))

// gitDirent implements unixfs.FSCursorDirent for git tree entries.
type gitDirent struct {
	name      string
	isDir     bool
	isFile    bool
	isSymlink bool
}

// GetName returns the name of the directory entry.
func (d *gitDirent) GetName() string {
	return d.name
}

// GetIsDirectory returns if the node is a directory.
func (d *gitDirent) GetIsDirectory() bool {
	return d.isDir
}

// GetIsFile returns if the node is a regular file.
func (d *gitDirent) GetIsFile() bool {
	return d.isFile
}

// GetIsSymlink returns if the node is a symlink.
func (d *gitDirent) GetIsSymlink() bool {
	return d.isSymlink
}

// _ is a type assertion
var _ unixfs.FSCursorDirent = ((*gitDirent)(nil))
