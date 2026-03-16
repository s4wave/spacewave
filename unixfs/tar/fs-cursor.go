package unixfs_tar

import (
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// TarFSCursor implements unixfs.FSCursor backed by a parsed tar archive.
type TarFSCursor struct {
	isReleased atomic.Bool
	node       *tarNode
}

// NewTarFSCursor creates a read-only FSCursor from a tar archive.
// The ReaderAt must remain valid for the lifetime of the cursor.
// size is the total byte length of the tar data.
func NewTarFSCursor(ra io.ReaderAt, size int64) (*TarFSCursor, error) {
	root, err := parseTar(ra, size)
	if err != nil {
		return nil, err
	}
	return &TarFSCursor{node: root}, nil
}

// NewTarFSCursorFromReader reads the entire tar into memory, then
// creates a cursor backed by the in-memory buffer.
func NewTarFSCursorFromReader(r io.Reader) (*TarFSCursor, error) {
	root, _, err := parseTarFromReader(r)
	if err != nil {
		return nil, err
	}
	return &TarFSCursor{node: root}, nil
}

// newTarFSCursorFromNode wraps an existing tarNode.
func newTarFSCursorFromNode(node *tarNode) *TarFSCursor {
	return &TarFSCursor{node: node}
}

// CheckReleased checks if the cursor is released.
func (c *TarFSCursor) CheckReleased() bool {
	return c.isReleased.Load()
}

// GetProxyCursor returns nil, nil (no proxy needed for immutable data).
func (c *TarFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return nil, nil
}

// AddChangeCb is a no-op (immutable archive never changes).
func (c *TarFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {}

// GetCursorOps returns the FSCursorOps for this cursor.
func (c *TarFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return newTarFSCursorOps(c), nil
}

// Release releases the cursor.
func (c *TarFSCursor) Release() {
	c.isReleased.Store(true)
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*TarFSCursor)(nil))

// TarFSCursorOps implements unixfs.FSCursorOps for tar archive nodes.
type TarFSCursorOps struct {
	cursor *TarFSCursor
	node   *tarNode
}

// newTarFSCursorOps creates ops from a cursor.
func newTarFSCursorOps(c *TarFSCursor) *TarFSCursorOps {
	return &TarFSCursorOps{
		cursor: c,
		node:   c.node,
	}
}

// CheckReleased checks if the ops is released.
func (o *TarFSCursorOps) CheckReleased() bool {
	if o == nil {
		return true
	}
	return o.cursor.CheckReleased()
}

// GetName returns the name of the node.
func (o *TarFSCursorOps) GetName() string {
	return o.node.name
}

// GetIsDirectory returns if the node is a directory.
func (o *TarFSCursorOps) GetIsDirectory() bool {
	return o.node.isDir
}

// GetIsFile returns if the node is a regular file.
func (o *TarFSCursorOps) GetIsFile() bool {
	return !o.node.isDir && !o.node.isLink
}

// GetIsSymlink returns if the node is a symlink.
func (o *TarFSCursorOps) GetIsSymlink() bool {
	return o.node.isLink
}

// GetPermissions returns the file mode permissions.
func (o *TarFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return o.node.mode, nil
}

// SetPermissions returns ErrReadOnly.
func (o *TarFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// GetSize returns the size of the node in bytes.
func (o *TarFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.node.isDir {
		return 0, nil
	}
	return uint64(o.node.size), nil //nolint:gosec
}

// GetModTimestamp returns the modification timestamp from the tar header.
func (o *TarFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return o.node.modTime, nil
}

// SetModTimestamp returns ErrReadOnly.
func (o *TarFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// ReadAt reads from a file node at the given offset.
func (o *TarFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}

	if offset >= o.node.size {
		return 0, io.EOF
	}

	avail := o.node.size - offset
	readLen := min(int64(len(data)), avail)

	n, err := o.node.ra.ReadAt(data[:readLen], o.node.offset+offset)
	n64 := int64(n)
	if err == io.EOF && n64 == avail {
		return n64, io.EOF
	}
	if err != nil {
		return n64, err
	}
	if n64 < int64(len(data)) {
		return n64, io.EOF
	}
	return n64, nil
}

// GetOptimalWriteSize returns 0, ErrReadOnly.
func (o *TarFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 0, unixfs_errors.ErrReadOnly
}

// WriteAt returns ErrReadOnly.
func (o *TarFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Truncate returns ErrReadOnly.
func (o *TarFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Lookup looks up a child entry in a directory.
func (o *TarFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.node.isDir {
		return nil, unixfs_errors.ErrNotDirectory
	}

	child, ok := o.node.childMap[name]
	if !ok {
		return nil, unixfs_errors.ErrNotExist
	}
	return newTarFSCursorFromNode(child), nil
}

// ReaddirAll reads all directory entries.
func (o *TarFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.node.isDir {
		return unixfs_errors.ErrNotDirectory
	}

	for i := int(skip); i < len(o.node.children); i++ { //nolint:gosec
		child := o.node.children[i]
		d := &tarDirent{
			name:   child.name,
			isDir:  child.isDir,
			isLink: child.isLink,
		}
		if err := cb(d); err != nil {
			return err
		}
	}
	return nil
}

// Mknod returns ErrReadOnly.
func (o *TarFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Symlink returns ErrReadOnly.
func (o *TarFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, tgtIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Readlink reads a symbolic link's target.
func (o *TarFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}

	if name == "" {
		if !o.node.isLink {
			return nil, false, unixfs_errors.ErrNotSymlink
		}
		isAbsolute := path.IsAbs(o.node.linkTgt)
		tgt := strings.TrimPrefix(o.node.linkTgt, "/")
		return strings.Split(tgt, "/"), isAbsolute, nil
	}

	if !o.node.isDir {
		return nil, false, unixfs_errors.ErrNotDirectory
	}
	child, ok := o.node.childMap[name]
	if !ok {
		return nil, false, unixfs_errors.ErrNotExist
	}
	if !child.isLink {
		return nil, false, unixfs_errors.ErrNotSymlink
	}
	isAbsolute := path.IsAbs(child.linkTgt)
	tgt := strings.TrimPrefix(child.linkTgt, "/")
	return strings.Split(tgt, "/"), isAbsolute, nil
}

// CopyTo returns false, nil (not implemented).
func (o *TarFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

// CopyFrom returns false, nil (not implemented).
func (o *TarFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

// MoveTo returns false, ErrReadOnly.
func (o *TarFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// MoveFrom returns false, ErrReadOnly.
func (o *TarFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, unixfs_errors.ErrReadOnly
}

// Remove returns ErrReadOnly.
func (o *TarFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// MknodWithContent returns ErrReadOnly.
func (o *TarFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*TarFSCursorOps)(nil))

// tarDirent implements unixfs.FSCursorDirent for tar entries.
type tarDirent struct {
	name   string
	isDir  bool
	isLink bool
}

// GetName returns the name of the directory entry.
func (d *tarDirent) GetName() string {
	return d.name
}

// GetIsDirectory returns if the node is a directory.
func (d *tarDirent) GetIsDirectory() bool {
	return d.isDir
}

// GetIsFile returns if the node is a regular file.
func (d *tarDirent) GetIsFile() bool {
	return !d.isDir && !d.isLink
}

// GetIsSymlink returns if the node is a symlink.
func (d *tarDirent) GetIsSymlink() bool {
	return d.isLink
}

// _ is a type assertion
var _ unixfs.FSCursorDirent = ((*tarDirent)(nil))
