package unixfs_git

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/format/objfile"
	go_git_storer "github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

const dotGitDefaultDescription = "Unnamed repository; edit this file 'description' to name the repository.\n"

// DotGitFSCursorOps implements unixfs.FSCursorOps for .git layout nodes.
type DotGitFSCursorOps struct {
	isReleased atomic.Bool
	cursor     *DotGitFSCursor
	node       *dotGitNode
}

func newDotGitFSCursorOps(c *DotGitFSCursor) *DotGitFSCursorOps {
	return &DotGitFSCursorOps{
		cursor: c,
		node:   c.node,
	}
}

// CheckReleased checks if the ops is released.
func (o *DotGitFSCursorOps) CheckReleased() bool {
	if o == nil {
		return true
	}
	return o.isReleased.Load() || o.cursor.CheckReleased()
}

// GetName returns the name of the node.
func (o *DotGitFSCursorOps) GetName() string {
	return o.node.name
}

// GetIsDirectory returns if the node is a directory.
func (o *DotGitFSCursorOps) GetIsDirectory() bool {
	return o.node.kind == dotGitNodeKindDir
}

// GetIsFile returns if the node is a regular file.
func (o *DotGitFSCursorOps) GetIsFile() bool {
	return o.node.kind == dotGitNodeKindFile
}

// GetIsSymlink returns false because .git seed nodes are not symlinks.
func (o *DotGitFSCursorOps) GetIsSymlink() bool {
	return false
}

// GetPermissions returns the permissions for this node.
func (o *DotGitFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if o.GetIsDirectory() {
		return 0o755 | fs.ModeDir, nil
	}
	return 0o644, nil
}

// SetPermissions rejects or stages a permission change.
func (o *DotGitFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	return o.writeError()
}

// GetSize returns the node size in bytes.
func (o *DotGitFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	// Require a live .git cursor before reading its size.
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	// Treat .git directories as empty content.
	if o.GetIsDirectory() {
		return 0, nil
	}

	// Load the .git file content to measure its size.
	content, err := o.content(ctx)
	if err != nil {
		return 0, err
	}
	return uint64(len(content)), nil
}

// GetModTimestamp returns the modification timestamp.
func (o *DotGitFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return time.Time{}, nil
}

// SetModTimestamp rejects or stages a timestamp change.
func (o *DotGitFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return o.writeError()
}

// ReadAt reads file content from the node.
func (o *DotGitFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	// Require a live .git file and a nonnegative read offset.
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}
	if offset < 0 {
		return 0, errors.New("negative offset")
	}

	// Load the .git content and detect reads past its end.
	content, err := o.content(ctx)
	if err != nil {
		return 0, err
	}
	if offset >= int64(len(content)) {
		return 0, io.EOF
	}

	// Copy the requested .git bytes and report a short read.
	n := copy(data, content[offset:])
	if n < len(data) {
		return int64(n), io.EOF
	}
	return int64(n), nil
}

// GetOptimalWriteSize returns the preferred write size.
func (o *DotGitFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	if err := o.checkWritable(); err != nil {
		return 0, err
	}
	return 0, nil
}

// WriteAt rejects or stages file content bytes.
func (o *DotGitFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Require a writable .git file and a nonnegative write offset.
	if err := o.checkWritable(); err != nil {
		return err
	}
	if !o.GetIsFile() {
		return unixfs_errors.ErrNotFile
	}
	if offset < 0 {
		return errors.New("negative offset")
	}

	// Load the .git content before applying the write.
	content, err := o.content(ctx)
	if err != nil {
		return err
	}

	// Validate the end of the .git write against offset overflow.
	end := int(offset) + len(data) //nolint:gosec
	if int64(end) < offset {
		return errors.New("write offset overflow")
	}

	// Extend the .git content to accommodate the write.
	if len(content) < end {
		ncontent := make([]byte, end)
		copy(ncontent, content)
		content = ncontent
	}

	// Apply the .git bytes and persist the complete content.
	copy(content[int(offset):], data)
	return o.writeFullContent(ctx, content)
}

// Truncate rejects or stages file truncation.
func (o *DotGitFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	// Require a writable .git file before changing its length.
	if err := o.checkWritable(); err != nil {
		return err
	}
	if !o.GetIsFile() {
		return unixfs_errors.ErrNotFile
	}

	// Load the .git content before resizing it.
	content, err := o.content(ctx)
	if err != nil {
		return err
	}

	// Resize the .git content to the requested length.
	if uint64(len(content)) > nsize {
		content = content[:nsize]
	} else if uint64(len(content)) < nsize {
		ncontent := make([]byte, nsize)
		copy(ncontent, content)
		content = ncontent
	}
	return o.writeFullContent(ctx, content)
}

// Lookup looks up a child entry in a directory.
func (o *DotGitFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	// Require a live .git directory before looking up a child.
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	// Resolve a staged .git child before stored entries.
	if child, ok := o.cursor.writeState.lookup(o.node.path, name); ok {
		return newDotGitFSCursorFromNode(o.cursor.tx, child, o.cursor.writable, o.cursor.changeSource, o.cursor.writeState), nil
	}

	// Resolve a loose Git object from the child name.
	if child, ok, err := o.lookupObject(name); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return newDotGitFSCursorFromNode(o.cursor.tx, child, o.cursor.writable, o.cursor.changeSource, o.cursor.writeState), nil
	}

	// Resolve a Git reference from the child name.
	if child, ok, err := o.lookupRef(name); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return newDotGitFSCursorFromNode(o.cursor.tx, child, o.cursor.writable, o.cursor.changeSource, o.cursor.writeState), nil
	}

	// Resolve the child from the materialized .git layout.
	child := o.node.child(name)
	if child == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	return newDotGitFSCursorFromNode(o.cursor.tx, child, o.cursor.writable, o.cursor.changeSource, o.cursor.writeState), nil
}

// ReaddirAll reads all directory entries.
func (o *DotGitFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	// Require a live .git directory before enumerating children.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}

	// Enumerate stored Git objects with staged entries overlaid.
	if ents, ok, err := o.readObjectsDir(); ok || err != nil {
		if err != nil {
			return err
		}
		ents = dotGitInfoToDirents(o.cursor.writeState.overlayDirents(o.node.path, dotGitDirentsToInfo(ents)))
		for i := int(skip); i < len(ents); i++ { //nolint:gosec
			if err := cb(ents[i]); err != nil {
				return err
			}
		}
		return nil
	}

	// Enumerate stored Git references with staged entries overlaid.
	if ents, ok, err := o.readRefsDir(); ok || err != nil {
		if err != nil {
			return err
		}
		ents = dotGitInfoToDirents(o.cursor.writeState.overlayDirents(o.node.path, dotGitDirentsToInfo(ents)))
		for i := int(skip); i < len(ents); i++ { //nolint:gosec
			if err := cb(ents[i]); err != nil {
				return err
			}
		}
		return nil
	}

	// Collect the materialized .git children for the directory overlay.
	infos := make([]unixfsDirentInfo, 0, len(o.node.children))
	for _, child := range o.node.children {
		infos = append(infos, unixfsDirentInfo{
			name:   child.name,
			isDir:  child.kind == dotGitNodeKindDir,
			isFile: child.kind == dotGitNodeKindFile,
		})
	}

	// Deliver the overlaid .git entries after the requested skip.
	ents := dotGitInfoToDirents(o.cursor.writeState.overlayDirents(o.node.path, infos))
	for i := int(skip); i < len(ents); i++ { //nolint:gosec
		if err := cb(ents[i]); err != nil {
			return err
		}
	}
	return nil
}

// Mknod rejects or stages node creation.
func (o *DotGitFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if err := o.checkWritable(); err != nil {
		return err
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	for _, name := range names {
		path := append(slices.Clone(o.node.path), name)
		if checkExist {
			if _, err := o.Lookup(ctx, name); err == nil {
				return unixfs_errors.ErrExist
			} else if err != unixfs_errors.ErrNotExist {
				return err
			}
		}
		if nodeType.GetIsDirectory() && dotGitPathIsObjectTemp(path) {
			o.cursor.writeState.setDir(path)
			continue
		}
		if !nodeType.GetIsFile() {
			return ErrDotGitWriteNotImplemented
		}
		if dotGitPathIsObjectTemp(path) {
			o.cursor.writeState.set(path, nil)
			continue
		}
		if dotGitPathIsReferenceLock(path) {
			o.cursor.writeState.set(path, nil)
			continue
		}
		if dotGitPathIsReference(path) {
			return o.applyRefContent(ctx, path, nil)
		}
		return ErrDotGitWriteNotImplemented
	}
	return nil
}

// Symlink rejects or stages symlink creation.
func (o *DotGitFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, tgtIsAbsolute bool, ts time.Time) error {
	return o.writeError()
}

// Readlink returns ErrNotSymlink.
func (o *DotGitFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}
	return nil, false, unixfs_errors.ErrNotSymlink
}

// CopyTo returns false, nil.
func (o *DotGitFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, nil
}

// CopyFrom returns false, nil.
func (o *DotGitFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

// MoveTo rejects or stages a move into another directory.
func (o *DotGitFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	return false, o.writeError()
}

// MoveFrom rejects or stages a move from another cursor.
func (o *DotGitFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	// Require a writable .git cursor before moving content.
	if err := o.checkWritable(); err != nil {
		return false, err
	}

	// Limit .git moves to files from another .git cursor.
	srcOps, ok := srcCursorOps.(*DotGitFSCursorOps)
	if !ok {
		return false, nil
	}
	if !o.GetIsDirectory() || !srcOps.GetIsFile() {
		return false, nil
	}

	// Publish a temporary Git object at its final loose-object path.
	dstPath := append(slices.Clone(o.node.path), name)
	if dotGitPathIsLooseObject(dstPath) && dotGitPathIsObjectTemp(srcOps.node.path) {
		content, err := srcOps.content(ctx)
		if err != nil {
			return false, err
		}
		if err := o.applyObjectContent(ctx, dstPath, content); err != nil {
			return false, err
		}
		o.cursor.writeState.remove(srcOps.node.path)
		return true, nil
	}

	// Require a reference lock whose target matches the destination.
	if !dotGitPathIsReference(dstPath) {
		return false, nil
	}
	if !dotGitPathIsReferenceLock(srcOps.node.path) || !slices.Equal(dotGitReferenceLockTarget(srcOps.node.path), dstPath) {
		return false, ErrDotGitWriteNotImplemented
	}

	// Read the staged Git reference content.
	content, err := srcOps.content(ctx)
	if err != nil {
		return false, err
	}

	// Publish the Git reference and remove its staged lock.
	if err := o.applyRefContent(ctx, dstPath, content); err != nil {
		return false, err
	}
	o.cursor.writeState.remove(srcOps.node.path)
	return true, nil
}

// Remove rejects or stages entry removal.
func (o *DotGitFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if err := o.checkWritable(); err != nil {
		return err
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	for _, name := range names {
		path := append(slices.Clone(o.node.path), name)
		if dotGitPathIsReferenceLock(path) {
			o.cursor.writeState.remove(path)
			continue
		}
		if dotGitPathIsObjectTemp(path) {
			o.cursor.writeState.remove(path)
			continue
		}
		if dotGitPathIsReference(path) {
			refName := dotGitReferenceNameFromPath(path)
			if err := o.cursor.tx.RemoveReference(refName); err != nil {
				return err
			}
			if err := o.commitAndInvalidate(ctx); err != nil {
				return err
			}
			continue
		}
		return ErrDotGitWriteNotImplemented
	}
	return nil
}

// MknodWithContent rejects or stages file creation with content.
func (o *DotGitFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	// Require a writable .git directory and a regular file.
	if err := o.checkWritable(); err != nil {
		return err
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	if !nodeType.GetIsFile() {
		return ErrDotGitWriteNotImplemented
	}

	// Read the .git file content and verify its declared length.
	data, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}
	if dataLen >= 0 && int64(len(data)) != dataLen {
		return errors.Errorf("expected %d bytes got %d", dataLen, len(data))
	}

	// Stage temporary .git content before publishing durable files.
	path := append(slices.Clone(o.node.path), name)
	if dotGitPathIsReferenceLock(path) {
		o.cursor.writeState.set(path, data)
		return nil
	}
	if dotGitPathIsObjectTemp(path) {
		o.cursor.writeState.set(path, data)
		return nil
	}

	// Publish Git objects, metadata, or references through the transaction.
	if dotGitPathIsLooseObject(path) {
		return o.applyObjectContent(ctx, path, data)
	}
	if dotGitPathIsMetadataFile(path) {
		return o.applyMetadataContent(ctx, path, data)
	}
	if dotGitPathIsReference(path) {
		return o.applyRefContent(ctx, path, data)
	}
	return ErrDotGitWriteNotImplemented
}

func (o *DotGitFSCursorOps) content(ctx context.Context) ([]byte, error) {
	if content, ok := o.cursor.writeState.get(o.node.path); ok {
		return content, nil
	}
	if o.node.hash != plumbing.ZeroHash {
		return dotGitObjectContent(o.cursor.tx, o.node.hash)
	}
	switch {
	case slices.Equal(o.node.path, []string{"HEAD"}):
		return dotGitHeadContent(o.cursor.tx)
	case slices.Equal(o.node.path, []string{"config"}):
		return dotGitConfigContent(o.cursor.tx)
	case slices.Equal(o.node.path, []string{"description"}):
		return []byte(dotGitDefaultDescription), nil
	case slices.Equal(o.node.path, []string{"objects", "info", "packs"}):
		return dotGitObjectsInfoPacksContent(o.cursor.tx)
	case slices.Equal(o.node.path, []string{"packed-refs"}):
		return dotGitPackedRefsContent(o.cursor.tx)
	case slices.Equal(o.node.path, []string{"shallow"}):
		return dotGitShallowContent(o.cursor.tx)
	default:
		return o.node.content, nil
	}
}

func (o *DotGitFSCursorOps) writeError() error {
	if err := o.checkWritable(); err != nil {
		return err
	}
	return ErrDotGitWriteNotImplemented
}

func (o *DotGitFSCursorOps) checkWritable() error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.cursor.writable {
		return unixfs_errors.ErrReadOnly
	}
	return nil
}

func (o *DotGitFSCursorOps) writeFullContent(ctx context.Context, content []byte) error {
	// Stage Git reference locks until their final move.
	if dotGitPathIsReferenceLock(o.node.path) {
		o.cursor.writeState.set(o.node.path, content)
		return nil
	}

	// Stage temporary Git objects until their final move.
	if dotGitPathIsObjectTemp(o.node.path) {
		o.cursor.writeState.set(o.node.path, content)
		return nil
	}

	// Publish complete Git objects, metadata, or reference content.
	if dotGitPathIsLooseObject(o.node.path) {
		return o.applyObjectContent(ctx, o.node.path, content)
	}
	if dotGitPathIsMetadataFile(o.node.path) {
		return o.applyMetadataContent(ctx, o.node.path, content)
	}
	if dotGitPathIsReference(o.node.path) {
		return o.applyRefContent(ctx, o.node.path, content)
	}
	return ErrDotGitWriteNotImplemented
}

func (o *DotGitFSCursorOps) applyRefContent(ctx context.Context, path []string, content []byte) error {
	// Parse the Git reference content and detect a removal.
	ref, remove, err := dotGitParseReferenceContent(dotGitReferenceNameFromPath(path), content)
	if err != nil {
		return err
	}

	// Remove an empty Git reference and commit the transaction.
	if remove {
		if err := o.cursor.tx.RemoveReference(dotGitReferenceNameFromPath(path)); err != nil {
			return err
		}
		return o.commitAndInvalidate(ctx)
	}

	// Write the parsed Git reference before committing it.
	if err := o.cursor.tx.SetReference(ref); err != nil {
		return err
	}
	return o.commitAndInvalidate(ctx)
}

func (o *DotGitFSCursorOps) applyMetadataContent(ctx context.Context, path []string, content []byte) error {
	switch {
	case slices.Equal(path, []string{"HEAD"}):
		ref, remove, err := dotGitParseReferenceContent(plumbing.HEAD, content)
		if err != nil {
			return err
		}
		if remove {
			if err := o.cursor.tx.RemoveReference(plumbing.HEAD); err != nil {
				return err
			}
			return o.commitAndInvalidate(ctx)
		}
		if err := o.cursor.tx.SetReference(ref); err != nil {
			return err
		}
		return o.commitAndInvalidate(ctx)
	case slices.Equal(path, []string{"config"}):
		cfg, err := dotGitParseConfigContent(content)
		if err != nil {
			return err
		}
		if err := o.cursor.tx.SetConfig(cfg); err != nil {
			return err
		}
		return o.commitAndInvalidate(ctx)
	case slices.Equal(path, []string{"shallow"}):
		hashes, err := dotGitParseShallowContent(content)
		if err != nil {
			return err
		}
		if err := o.cursor.tx.SetShallow(hashes); err != nil {
			return err
		}
		return o.commitAndInvalidate(ctx)
	case slices.Equal(path, []string{"packed-refs"}):
		refs, err := dotGitParsePackedRefsContent(content)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if err := o.cursor.tx.SetReference(ref); err != nil {
				return err
			}
		}
		return o.commitAndInvalidate(ctx)
	default:
		return ErrDotGitWriteNotImplemented
	}
}

func (o *DotGitFSCursorOps) applyObjectContent(ctx context.Context, path []string, content []byte) error {
	// Open the loose Git object reader for the supplied content.
	reader, err := objfile.NewReader(bytes.NewReader(content), formatcfg.SHA1)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Read the loose Git object type and declared size.
	typ, size, err := reader.Header()
	if err != nil {
		return err
	}

	// Prepare an encoded Git object and its content writer.
	obj := o.cursor.tx.NewEncodedObject()
	obj.SetType(typ)
	obj.SetSize(size)
	writer, err := obj.Writer()
	if err != nil {
		return err
	}

	// Copy the loose Git object content and close its writer.
	if _, err := io.Copy(writer, reader); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	// Verify the encoded Git object hash against its destination path.
	hash := obj.Hash()
	expected := dotGitLooseObjectHash(path)
	if hash != expected {
		return errors.Errorf("object hash %s does not match path %s", hash.String(), expected.String())
	}

	// Store the verified Git object before committing it.
	if _, err := o.cursor.tx.SetEncodedObject(obj); err != nil {
		return err
	}
	return o.commitAndInvalidate(ctx)
}

func (o *DotGitFSCursorOps) commitAndInvalidate(ctx context.Context) error {
	if err := o.cursor.tx.Commit(ctx); err != nil {
		return err
	}
	o.cursor.Release()
	return nil
}

func (o *DotGitFSCursorOps) lookupObject(name string) (*dotGitNode, bool, error) {
	// Resolve a Git object prefix directory from stored object hashes.
	if slices.Equal(o.node.path, []string{"objects"}) {
		hashes, err := dotGitObjectHashes(o.cursor.tx)
		if err != nil {
			return nil, true, err
		}
		for _, hash := range hashes {
			if hash.String()[:2] == name {
				return newDotGitDirNode(name, []string{"objects", name}), true, nil
			}
		}
		return nil, false, nil
	}

	// Require a loose-object directory and a valid child hash.
	if len(o.node.path) != 2 || !slices.Equal(o.node.path[:1], []string{"objects"}) {
		return nil, false, nil
	}
	hash := o.node.path[1] + name
	if !plumbing.IsHash(hash) {
		return nil, false, nil
	}

	// Verify that the loose Git object exists before exposing its cursor.
	objHash := plumbing.NewHash(hash)
	if err := o.cursor.tx.HasEncodedObject(objHash); err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return nil, false, nil
		}
		return nil, true, err
	}
	return newDotGitObjectFileNode(objHash), true, nil
}

func (o *DotGitFSCursorOps) readObjectsDir() ([]unixfs.FSCursorDirent, bool, error) {
	// Limit object enumeration to Git object directories.
	if !slices.Equal(o.node.path, []string{"objects"}) && (len(o.node.path) != 2 || !slices.Equal(o.node.path[:1], []string{"objects"}) || !dotGitIsLooseObjectPrefix(o.node.path[1])) {
		return nil, false, nil
	}

	// Collect stored Git object hashes for directory entries.
	hashes, err := dotGitObjectHashes(o.cursor.tx)
	if err != nil {
		return nil, true, err
	}

	// Select prefix directories or object suffix files for this Git path.
	if slices.Equal(o.node.path, []string{"objects"}) {
		return dotGitObjectPrefixDirents(o.node.children, hashes), true, nil
	}
	return dotGitObjectSuffixDirents(o.node.path[1], hashes), true, nil
}

func (o *DotGitFSCursorOps) lookupRef(name string) (*dotGitNode, bool, error) {
	switch {
	case slices.Equal(o.node.path, []string{"refs"}):
		switch name {
		case "heads":
			return newDotGitDirNode(name, []string{"refs", "heads"}), true, nil
		case "tags":
			return newDotGitDirNode(name, []string{"refs", "tags"}), true, nil
		default:
			return nil, true, unixfs_errors.ErrNotExist
		}
	case dotGitRefsPathKind(o.node.path) != "":
		return o.lookupRefBelowKind(name)
	default:
		return nil, false, nil
	}
}

func (o *DotGitFSCursorOps) lookupRefBelowKind(name string) (*dotGitNode, bool, error) {
	// Collect Git references beneath the current kind and prefix.
	kind := dotGitRefsPathKind(o.node.path)
	prefix := o.node.path[2:]
	refs, err := dotGitCollectRefs(o.cursor.tx, kind, prefix)
	if err != nil {
		return nil, true, err
	}

	// Resolve the requested child as a Git reference file or directory.
	for _, ref := range refs {
		parts, ok := dotGitReferencePath(ref, kind)
		if !ok || len(parts) <= len(prefix) || !slices.Equal(parts[:len(prefix)], prefix) {
			continue
		}
		next := parts[len(prefix)]
		if next != name {
			continue
		}
		path := append(append([]string{}, o.node.path...), name)
		if len(parts) == len(prefix)+1 {
			content := []byte(dotGitReferenceFileContent(ref))
			return newDotGitFileNode(name, path, content), true, nil
		}
		return newDotGitDirNode(name, path), true, nil
	}
	return nil, true, unixfs_errors.ErrNotExist
}

func (o *DotGitFSCursorOps) readRefsDir() ([]unixfs.FSCursorDirent, bool, error) {
	switch {
	case slices.Equal(o.node.path, []string{"refs"}):
		return []unixfs.FSCursorDirent{
			&gitDirent{name: "heads", isDir: true},
			&gitDirent{name: "tags", isDir: true},
		}, true, nil
	case dotGitRefsPathKind(o.node.path) != "":
		ents, err := o.readRefKindDir()
		return ents, true, err
	default:
		return nil, false, nil
	}
}

func (o *DotGitFSCursorOps) readRefKindDir() ([]unixfs.FSCursorDirent, error) {
	// Collect Git references beneath the current directory prefix.
	kind := dotGitRefsPathKind(o.node.path)
	prefix := o.node.path[2:]
	refs, err := dotGitCollectRefs(o.cursor.tx, kind, prefix)
	if err != nil {
		return nil, err
	}

	// Merge Git references into immediate file and directory entries.
	type ent struct {
		isDir bool
	}
	seen := make(map[string]ent)
	for _, ref := range refs {
		parts, ok := dotGitReferencePath(ref, kind)
		if !ok || len(parts) <= len(prefix) || !slices.Equal(parts[:len(prefix)], prefix) {
			continue
		}
		name := parts[len(prefix)]
		prev := seen[name]
		if len(parts) > len(prefix)+1 {
			prev.isDir = true
		}
		seen[name] = prev
	}

	// Sort the merged Git reference names for enumeration.
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)

	// Build Git directory entries from the sorted reference names.
	ents := make([]unixfs.FSCursorDirent, 0, len(names))
	for _, name := range names {
		info := seen[name]
		ents = append(ents, &gitDirent{
			name:   name,
			isDir:  info.isDir,
			isFile: !info.isDir,
		})
	}
	return ents, nil
}

func dotGitObjectHashes(storer go_git_storer.EncodedObjectStorer) ([]plumbing.Hash, error) {
	// Open the stored Git object iterator for hash collection.
	iter, err := storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Collect the hash of every stored Git object.
	var hashes []plumbing.Hash
	err = iter.ForEach(func(obj plumbing.EncodedObject) error {
		hashes = append(hashes, obj.Hash())
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort the Git object hashes for deterministic enumeration.
	plumbing.HashesSort(hashes)
	return hashes, nil
}

func dotGitObjectPrefixDirents(children []*dotGitNode, hashes []plumbing.Hash) []unixfs.FSCursorDirent {
	// Retain the materialized Git object directory names.
	seen := make(map[string]struct{})
	var names []string
	for _, child := range children {
		seen[child.name] = struct{}{}
		names = append(names, child.name)
	}

	// Add each unseen loose Git object prefix directory.
	for _, hash := range hashes {
		name := hash.String()[:2]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	// Sort Git object directory names and build their entries.
	slices.Sort(names)
	ents := make([]unixfs.FSCursorDirent, 0, len(names))
	for _, name := range names {
		ents = append(ents, &gitDirent{name: name, isDir: true})
	}
	return ents
}

func dotGitIsLooseObjectPrefix(prefix string) bool {
	if len(prefix) != 2 {
		return false
	}
	for _, ch := range prefix {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func dotGitObjectSuffixDirents(prefix string, hashes []plumbing.Hash) []unixfs.FSCursorDirent {
	// Collect loose Git object suffixes beneath the requested prefix.
	var names []string
	for _, hash := range hashes {
		hashStr := hash.String()
		if strings.HasPrefix(hashStr, prefix) {
			names = append(names, hashStr[2:])
		}
	}

	// Sort loose Git object names and build file entries.
	slices.Sort(names)
	ents := make([]unixfs.FSCursorDirent, 0, len(names))
	for _, name := range names {
		ents = append(ents, &gitDirent{name: name, isFile: true})
	}
	return ents
}

func dotGitObjectContent(storer go_git_storer.EncodedObjectStorer, hash plumbing.Hash) ([]byte, error) {
	// Resolve the stored Git object by its hash.
	obj, err := storer.EncodedObject(plumbing.AnyObject, hash)
	if err != nil {
		return nil, err
	}

	// Open the Git object content reader for loose-object encoding.
	reader, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Write the loose Git object header into a buffer.
	var buf bytes.Buffer
	writer := objfile.NewWriter(&buf, formatcfg.SHA1)
	if err := writer.WriteHeader(obj.Type(), obj.Size()); err != nil {
		return nil, err
	}

	// Encode the Git object content and finish its compressed stream.
	if _, err := io.Copy(writer, reader); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dotGitObjectsInfoPacksContent(storer any) ([]byte, error) {
	// Use an empty pack listing when the Git store has no pack support.
	packed, ok := storer.(go_git_storer.PackedObjectStorer)
	if !ok {
		return []byte("\n"), nil
	}

	// Read the Git store pack hashes.
	packs, err := packed.ObjectPacks()
	if err != nil {
		return nil, err
	}

	// Render the sorted Git pack hashes in the info/packs format.
	plumbing.HashesSort(packs)
	var buf bytes.Buffer
	for _, pack := range packs {
		buf.WriteString("P pack-")
		buf.WriteString(pack.String())
		buf.WriteString(".pack\n")
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func dotGitHeadContent(storer interface {
	Reference(plumbing.ReferenceName) (*plumbing.Reference, error)
},
) ([]byte, error) {
	// Use the default Git HEAD when no reference store is available.
	if storer == nil {
		return []byte("ref: refs/heads/master\n"), nil
	}

	// Read Git HEAD and use the default for an unborn repository.
	ref, err := storer.Reference(plumbing.HEAD)
	if err == plumbing.ErrReferenceNotFound {
		return []byte("ref: refs/heads/master\n"), nil
	}
	if err != nil {
		return nil, err
	}

	// Render the Git HEAD target as file content.
	parts := ref.Strings()
	return []byte(parts[1] + "\n"), nil
}

func dotGitConfigContent(storer interface {
	Config() (*config.Config, error)
},
) ([]byte, error) {
	cfg := config.NewConfig()
	if storer != nil {
		var err error
		cfg, err = storer.Config()
		if err != nil {
			return nil, err
		}
	}
	return cfg.Marshal()
}

func dotGitReferenceFileContent(ref *plumbing.Reference) string {
	parts := ref.Strings()
	return parts[1] + "\n"
}

func dotGitPackedRefsContent(refStorer interface {
	IterReferences() (go_git_storer.ReferenceIter, error)
},
) ([]byte, error) {
	// Use empty packed references when no Git reference store exists.
	if refStorer == nil {
		return nil, nil
	}

	// Open the Git reference iterator for packed-ref rendering.
	iter, err := refStorer.IterReferences()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Collect hash references beneath the Git refs namespace.
	var lines []string
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference || !strings.HasPrefix(ref.Name().String(), "refs/") {
			return nil
		}
		lines = append(lines, ref.Hash().String()+" "+ref.Name().String()+"\n")
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Render sorted Git references with the packed-refs header.
	slices.Sort(lines)
	var buf bytes.Buffer
	buf.WriteString("# pack-refs with: peeled fully-peeled sorted \n")
	for _, line := range lines {
		buf.WriteString(line)
	}
	return buf.Bytes(), nil
}

func dotGitShallowContent(storer interface {
	Shallow() ([]plumbing.Hash, error)
},
) ([]byte, error) {
	// Use empty shallow content when no Git store is available.
	if storer == nil {
		return nil, nil
	}

	// Read the Git shallow boundary hashes.
	hashes, err := storer.Shallow()
	if err != nil {
		return nil, err
	}

	// Render sorted Git shallow hashes as newline-delimited content.
	plumbing.HashesSort(hashes)
	var buf bytes.Buffer
	for _, hash := range hashes {
		buf.WriteString(hash.String())
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func dotGitRefsPathKind(path []string) string {
	if len(path) < 2 || path[0] != "refs" {
		return ""
	}
	switch path[1] {
	case "heads":
		return "heads"
	case "tags":
		return "tags"
	default:
		return ""
	}
}

func dotGitCollectRefs(refStorer interface {
	IterReferences() (go_git_storer.ReferenceIter, error)
}, kind string, prefix []string,
) ([]*plumbing.Reference, error) {
	// Use an empty reference collection when no Git store exists.
	if refStorer == nil {
		return nil, nil
	}

	// Open the Git reference iterator for prefix filtering.
	iter, err := refStorer.IterReferences()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Collect Git references beneath the requested kind and prefix.
	var refs []*plumbing.Reference
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		parts, ok := dotGitReferencePath(ref, kind)
		if ok && len(parts) > len(prefix) && slices.Equal(parts[:len(prefix)], prefix) {
			refs = append(refs, ref)
		}
		return nil
	})
	return refs, err
}

func dotGitReferencePath(ref *plumbing.Reference, kind string) ([]string, bool) {
	// Select the Git reference namespace for the requested kind.
	var prefix string
	switch kind {
	case "heads":
		prefix = "refs/heads/"
	case "tags":
		prefix = "refs/tags/"
	default:
		return nil, false
	}

	// Require the Git reference name to belong to that namespace.
	name := ref.Name().String()
	if !strings.HasPrefix(name, prefix) {
		return nil, false
	}

	// Split the Git reference name into its remaining path components.
	rest := strings.TrimPrefix(name, prefix)
	if rest == "" {
		return nil, false
	}
	return strings.Split(rest, "/"), true
}

// _ is a type assertion
var _ unixfs.FSCursorOps = (*DotGitFSCursorOps)(nil)
