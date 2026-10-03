package unixfs_overlay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

const (
	whiteoutPrefix = ".wh."
	opaqueMarker   = ".wh..wh..opq"
)

// OverlayFSCursorOps implements overlay FSCursor operations.
type OverlayFSCursorOps struct {
	released atomic.Bool
	c        *OverlayFSCursor
	lowerOps unixfs.FSCursorOps
	upperOps unixfs.FSCursorOps
	active   unixfs.FSCursorOps
}

// CheckReleased implements FSCursorOps.
func (o *OverlayFSCursorOps) CheckReleased() bool {
	return o == nil || o.released.Load() || o.c.CheckReleased()
}

// GetName implements FSCursorOps.
func (o *OverlayFSCursorOps) GetName() string {
	return o.active.GetName()
}

// GetIsDirectory implements FSCursorOps.
func (o *OverlayFSCursorOps) GetIsDirectory() bool {
	return o.active.GetIsDirectory()
}

// GetIsFile implements FSCursorOps.
func (o *OverlayFSCursorOps) GetIsFile() bool {
	return o.active.GetIsFile()
}

// GetIsSymlink implements FSCursorOps.
func (o *OverlayFSCursorOps) GetIsSymlink() bool {
	return o.active.GetIsSymlink()
}

// GetPermissions implements FSCursorOps.
func (o *OverlayFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return o.active.GetPermissions(ctx)
}

// SetPermissions implements FSCursorOps.
func (o *OverlayFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	// Require a live overlay cursor before changing permissions.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Hold the overlay lock while copying up and changing permissions.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy the node to the upper layer and update its permissions.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.SetPermissions(ctx, permissions, ts)
}

// GetSize implements FSCursorOps.
func (o *OverlayFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return o.active.GetSize(ctx)
}

// GetModTimestamp implements FSCursorOps.
func (o *OverlayFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return o.active.GetModTimestamp(ctx)
}

// SetModTimestamp implements FSCursorOps.
func (o *OverlayFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	// Require a live overlay cursor before changing its timestamp.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Hold the overlay lock while copying up and changing the timestamp.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy the node to the upper layer and update its timestamp.
	upperOps, err := o.ensureUpperLocked(ctx, mtime)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.SetModTimestamp(ctx, mtime)
}

// ReadAt implements FSCursorOps.
func (o *OverlayFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return o.active.ReadAt(ctx, offset, data)
}

// GetOptimalWriteSize implements FSCursorOps.
func (o *OverlayFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !o.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}
	if o.upperOps == nil {
		return 0, nil
	}
	return o.upperOps.GetOptimalWriteSize(ctx)
}

// WriteAt implements FSCursorOps.
func (o *OverlayFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Require a live overlay cursor before writing file content.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Hold the overlay lock while copying up and writing file content.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy the file to the upper layer and write its content.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.WriteAt(ctx, offset, data, ts)
}

// Truncate implements FSCursorOps.
func (o *OverlayFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	// Require a live overlay cursor before changing the file size.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Hold the overlay lock while copying up and resizing the file.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy the file to the upper layer and change its size.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.Truncate(ctx, nsize, ts)
}

// Lookup implements FSCursorOps.
func (o *OverlayFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	// Require a live overlay directory before looking up a child.
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	// Hold the overlay lock while resolving child visibility.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Hide a child covered by an upper-layer whiteout.
	if hidden, err := hasUpperChildLocked(ctx, o.c.upper, whiteoutName(name)); err != nil {
		return nil, err
	} else if hidden {
		return nil, unixfs_errors.ErrNotExist
	}

	// Resolve the child cursors from both filesystem layers.
	upperChild, err := lookupChildLocked(ctx, o.c.upper, name)
	if err != nil && !isNotExist(err) {
		return nil, err
	}
	lowerChild, lowerErr := lookupChildLocked(ctx, o.c.lower, name)
	if lowerErr != nil && !isNotExist(lowerErr) {
		if upperChild != nil {
			upperChild.Release()
		}
		return nil, lowerErr
	}

	// Prefer the upper child and retain a lower child of the same type.
	if upperChild != nil {
		if lowerChild != nil {
			same, err := cursorsHaveSameType(ctx, upperChild, lowerChild)
			if err != nil {
				upperChild.Release()
				lowerChild.Release()
				return nil, err
			}
			if !same {
				lowerChild.Release()
				lowerChild = nil
			}
		}
		return newOverlayFSCursor(o.c.state, o.c, name, lowerChild, upperChild), nil
	}

	// Require a lower child visible through the upper directory.
	if lowerChild == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	if opaque, err := hasUpperChildLocked(ctx, o.c.upper, opaqueMarker); err != nil {
		lowerChild.Release()
		return nil, err
	} else if opaque {
		lowerChild.Release()
		return nil, unixfs_errors.ErrNotExist
	}

	return newOverlayFSCursor(o.c.state, o.c, name, lowerChild, nil), nil
}

// ReaddirAll implements FSCursorOps.
func (o *OverlayFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	// Require a live overlay directory and a callback for reading entries.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	if cb == nil {
		return nil
	}

	// Snapshot the visible overlay directory entries before invoking callbacks.
	ents, err := o.readdirSnapshotLocked(ctx)
	if err != nil {
		return err
	}

	// The overlay mutex must not be held while invoking the caller callback:
	// v86fs readdir re-enters Lookup/GetCursorOps, which re-locks it.
	for idx, ent := range ents {
		if uint64(idx) < skip {
			continue
		}
		if err := cb(ent); err != nil {
			return err
		}
	}
	return nil
}

func (o *OverlayFSCursorOps) readdirSnapshotLocked(ctx context.Context) ([]unixfs.FSCursorDirent, error) {
	// Hold the overlay lock while merging directory entries.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Prepare visible entries and upper-layer visibility markers.
	entries := map[string]unixfs.FSCursorDirent{}
	whiteouts := map[string]struct{}{}
	opaque := false

	// Collect upper entries and their whiteout and opacity markers.
	if o.upperOps != nil {
		err := o.upperOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
			// Classify the upper directory entry as a marker or visible node.
			name := ent.GetName()
			if name == opaqueMarker {
				opaque = true
				return nil
			}
			if after, ok := strings.CutPrefix(name, whiteoutPrefix); ok {
				whiteouts[after] = struct{}{}
				return nil
			}
			entries[name] = overlayDirentFrom(ent)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Merge lower entries when the upper directory is transparent.
	if o.lowerOps != nil && !opaque {
		err := o.lowerOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
			// Retain lower entries absent from the upper entries and whiteouts.
			name := ent.GetName()
			if _, ok := entries[name]; ok {
				return nil
			}
			if _, ok := whiteouts[name]; ok {
				return nil
			}
			entries[name] = overlayDirentFrom(ent)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Sort visible entry names for stable directory traversal.
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)

	// Build the directory entry snapshot in name order.
	ents := make([]unixfs.FSCursorDirent, 0, len(names))
	for _, name := range names {
		ents = append(ents, entries[name])
	}
	return ents, nil
}

// Mknod implements FSCursorOps.
func (o *OverlayFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	// Require a live overlay directory and names for node creation.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	if len(names) == 0 {
		return nil
	}

	// Hold the overlay lock while creating upper-layer nodes.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Reject node creation when a requested visible name already exists.
	if checkExist {
		for _, name := range names {
			exists, err := o.visibleChildExistsLocked(ctx, name)
			if err != nil {
				return err
			}
			if exists {
				return unixfs_errors.ErrExist
			}
		}
	}

	// Copy the directory up and remove whiteouts for the new nodes.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := upperOps.Remove(ctx, []string{whiteoutName(name)}, ts); err != nil {
			return err
		}
		upperOps, err = cursorOps(ctx, o.c.upper)
		if err != nil {
			return err
		}
	}
	o.released.Store(true)
	return upperOps.Mknod(ctx, false, names, nodeType, permissions, ts)
}

// Symlink implements FSCursorOps.
func (o *OverlayFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	// Require a live overlay directory before creating a symbolic link.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}

	// Hold the overlay lock while creating the symbolic link.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Reject symbolic link creation when its visible name already exists.
	if checkExist {
		exists, err := o.visibleChildExistsLocked(ctx, name)
		if err != nil {
			return err
		}
		if exists {
			return unixfs_errors.ErrExist
		}
	}

	// Copy the directory up and replace the whiteout with a symbolic link.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	if err := upperOps.Remove(ctx, []string{whiteoutName(name)}, ts); err != nil {
		return err
	}
	upperOps, err = cursorOps(ctx, o.c.upper)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.Symlink(ctx, false, name, target, targetIsAbsolute, ts)
}

// Readlink implements FSCursorOps.
func (o *OverlayFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}
	return o.active.Readlink(ctx, name)
}

// CopyTo implements FSCursorOps.
func (o *OverlayFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	return false, nil
}

// CopyFrom implements FSCursorOps.
func (o *OverlayFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, nil
}

// MoveTo implements FSCursorOps. It renames the source entry into the target
// directory under tgtName within the overlay: it copies the source and the
// target directory up, removes any target whiteout, performs the move inside
// the upper layer, then whiteouts the source name if the lower layer still
// exposes it. It returns done=false when the target is a different overlay so
// FSHandle.Rename can fall back. apt update relies on this to rename its cache
// temp file over the read-only lower copy.
//
// A directory that exists in the lower layer is copied up as an empty upper
// node before the move, so renaming such a directory does not carry forward
// lower children; the v86 consumers (apt) rename files, not lower-backed
// directories.
func (o *OverlayFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	// Require a live source and a target directory in the same overlay.
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	tgtOps, ok := tgtCursorOps.(*OverlayFSCursorOps)
	if !ok || tgtOps.c.state != o.c.state {
		return false, nil
	}
	if !tgtOps.GetIsDirectory() {
		return false, unixfs_errors.ErrNotDirectory
	}

	// Hold the overlay lock for the upper-layer move and source whiteout.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy source and target up and remove the target whiteout.
	srcUpperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return false, err
	}
	if err := tgtOps.c.ensureUpperLocked(ctx, ts); err != nil {
		return false, err
	}
	tgtUpperOps, err := cursorOps(ctx, tgtOps.c.upper)
	if err != nil {
		return false, err
	}
	if err := tgtUpperOps.Remove(ctx, []string{whiteoutName(tgtName)}, ts); err != nil {
		return false, err
	}
	tgtUpperOps, err = cursorOps(ctx, tgtOps.c.upper)
	if err != nil {
		return false, err
	}

	// Move the source node within the upper filesystem layer.
	done, err = srcUpperOps.MoveTo(ctx, tgtUpperOps, tgtName, ts)
	if err != nil {
		return false, err
	}
	if !done {
		return false, nil
	}

	// Hide the old source name when it still exists in the lower layer.
	if o.c.lower != nil && o.c.parent != nil {
		if err := o.c.parent.ensureUpperLocked(ctx, ts); err != nil {
			return false, err
		}
		parentUpperOps, err := cursorOps(ctx, o.c.parent.upper)
		if err != nil {
			return false, err
		}
		if err := parentUpperOps.MknodWithContent(
			ctx,
			whiteoutName(o.c.name),
			unixfs.NewFSCursorNodeType_File(),
			0,
			bytes.NewReader(nil),
			0,
			ts,
		); err != nil {
			return false, err
		}
	}

	// Invalidate the source operations after the completed move.
	o.released.Store(true)
	return true, nil
}

// MoveFrom implements FSCursorOps.
func (o *OverlayFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, nil
}

// Remove implements FSCursorOps.
func (o *OverlayFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	// Require a live overlay directory before removing children.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}

	// Hold the overlay lock while removing children and creating whiteouts.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Remove upper children and hide any matching lower children.
	for _, name := range names {
		// Resolve the child presence in both filesystem layers.
		lowerExists, err := hasChildLocked(ctx, o.c.lower, name)
		if err != nil {
			return err
		}
		upperExists, err := hasChildLocked(ctx, o.c.upper, name)
		if err != nil {
			return err
		}

		// Replace a lower-backed child with an upper-layer whiteout.
		if lowerExists {
			// Copy the directory up and remove any existing upper child.
			upperOps, err := o.ensureUpperLocked(ctx, ts)
			if err != nil {
				return err
			}
			if upperExists {
				if err := removeUpperEntryRecursiveLocked(ctx, upperOps, name, ts); err != nil {
					return err
				}
				upperOps, err = cursorOps(ctx, o.c.upper)
				if err != nil {
					return err
				}
			}

			// Replace the child whiteout marker after removing the upper entry.
			marker := whiteoutName(name)
			if err := upperOps.Remove(ctx, []string{marker}, ts); err != nil {
				return err
			}
			upperOps, err = cursorOps(ctx, o.c.upper)
			if err != nil {
				return err
			}
			if err := upperOps.MknodWithContent(
				ctx,
				marker,
				unixfs.NewFSCursorNodeType_File(),
				0,
				bytes.NewReader(nil),
				0,
				ts,
			); err != nil {
				return err
			}
			o.released.Store(true)
			continue
		}

		// Remove a child that exists only in the upper layer.
		if upperExists {
			upperOps, err := cursorOps(ctx, o.c.upper)
			if err != nil {
				return err
			}
			if err := removeUpperEntryRecursiveLocked(ctx, upperOps, name, ts); err != nil {
				return err
			}
			o.released.Store(true)
		}
	}

	return nil
}

// MknodWithContent implements FSCursorOps.
func (o *OverlayFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	// Require a live overlay directory before creating a file with content.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}

	// Hold the overlay lock for atomic upper-layer file creation.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()

	// Copy the directory up and replace the whiteout with the new file.
	upperOps, err := o.ensureUpperLocked(ctx, ts)
	if err != nil {
		return err
	}
	if err := upperOps.Remove(ctx, []string{whiteoutName(name)}, ts); err != nil {
		return err
	}
	upperOps, err = cursorOps(ctx, o.c.upper)
	if err != nil {
		return err
	}
	o.released.Store(true)
	return upperOps.MknodWithContent(ctx, name, nodeType, dataLen, rdr, permissions, ts)
}

func (o *OverlayFSCursorOps) ensureUpperLocked(ctx context.Context, ts time.Time) (unixfs.FSCursorOps, error) {
	if err := o.c.ensureUpperLocked(ctx, ts); err != nil {
		return nil, err
	}
	upperOps, err := cursorOps(ctx, o.c.upper)
	if err != nil {
		return nil, err
	}
	return upperOps, nil
}

func (o *OverlayFSCursorOps) visibleChildExistsLocked(ctx context.Context, name string) (bool, error) {
	if hidden, err := hasUpperChildLocked(ctx, o.c.upper, whiteoutName(name)); err != nil || hidden {
		return hidden, err
	}
	if ok, err := hasChildLocked(ctx, o.c.upper, name); err != nil || ok {
		return ok, err
	}
	if opaque, err := hasUpperChildLocked(ctx, o.c.upper, opaqueMarker); err != nil || opaque {
		return false, err
	}
	return hasChildLocked(ctx, o.c.lower, name)
}

func (c *OverlayFSCursor) ensureUpperLocked(ctx context.Context, ts time.Time) error {
	// Require a lower node and writable parent when the upper node is absent.
	if c.upper != nil {
		return nil
	}
	if c.parent == nil {
		return unixfs_errors.ErrReadOnly
	}
	if c.lower == nil {
		return unixfs_errors.ErrNotExist
	}
	if err := c.parent.ensureUpperLocked(ctx, ts); err != nil {
		return err
	}

	// Reuse an existing upper child beneath the copied-up parent.
	parentUpperOps, err := cursorOps(ctx, c.parent.upper)
	if err != nil {
		return err
	}
	if upperChild, err := parentUpperOps.Lookup(ctx, c.name); err == nil {
		c.upper = upperChild
		return nil
	} else if !isNotExist(err) {
		return err
	}

	// Read the lower node permissions and timestamp for copy-up.
	lowerOps, err := cursorOps(ctx, c.lower)
	if err != nil {
		return err
	}
	permissions, err := lowerOps.GetPermissions(ctx)
	if err != nil {
		return err
	}
	mtime, err := lowerOps.GetModTimestamp(ctx)
	if err != nil {
		return err
	}

	// Remove the child whiteout before creating its upper node.
	if err := parentUpperOps.Remove(ctx, []string{whiteoutName(c.name)}, ts); err != nil {
		return err
	}
	parentUpperOps, err = cursorOps(ctx, c.parent.upper)
	if err != nil {
		return err
	}

	// Create the upper node with the lower node type and content.
	switch {
	case lowerOps.GetIsDirectory():
		if err := parentUpperOps.Mknod(ctx, false, []string{c.name}, unixfs.NewFSCursorNodeType_Dir(), permissions, mtime); err != nil {
			return err
		}
	case lowerOps.GetIsFile():
		size, err := lowerOps.GetSize(ctx)
		if err != nil {
			return err
		}
		if size > math.MaxInt64 {
			return unixfs.NewReadFileSizeTooLargeError(size)
		}
		rdr := &cursorReader{ctx: ctx, ops: lowerOps, size: size}
		if err := parentUpperOps.MknodWithContent(
			ctx,
			c.name,
			unixfs.NewFSCursorNodeType_File(),
			int64(size),
			rdr,
			permissions,
			mtime,
		); err != nil {
			return err
		}
	case lowerOps.GetIsSymlink():
		target, isAbs, err := lowerOps.Readlink(ctx, "")
		if err != nil {
			return err
		}
		if err := parentUpperOps.Symlink(ctx, false, c.name, target, isAbs, mtime); err != nil {
			return err
		}
	default:
		return unixfs_errors.ErrNotExist
	}

	// Attach the newly copied-up child cursor.
	parentUpperOps, err = cursorOps(ctx, c.parent.upper)
	if err != nil {
		return err
	}
	upperChild, err := parentUpperOps.Lookup(ctx, c.name)
	if err != nil {
		return err
	}
	c.upper = upperChild

	// Restore the lower node timestamp on the upper copy.
	upperOps, err := cursorOps(ctx, c.upper)
	if err != nil {
		return err
	}
	_ = upperOps.SetModTimestamp(ctx, mtime)
	return nil
}

func (c *OverlayFSCursor) isHiddenByParentLocked(ctx context.Context) (bool, error) {
	if c.parent == nil {
		return false, nil
	}
	if hidden, err := hasUpperChildLocked(ctx, c.parent.upper, whiteoutName(c.name)); err != nil || hidden {
		return hidden, err
	}
	return hasUpperChildLocked(ctx, c.parent.upper, opaqueMarker)
}

func cursorOps(ctx context.Context, c unixfs.FSCursor) (unixfs.FSCursorOps, error) {
	// Resolve operations only for an existing live filesystem cursor.
	if c == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	ops, err := c.GetCursorOps(ctx)
	if err != nil {
		return nil, err
	}
	if ops == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	return ops, nil
}

func lookupChildLocked(ctx context.Context, parent unixfs.FSCursor, name string) (unixfs.FSCursor, error) {
	ops, err := cursorOps(ctx, parent)
	if err != nil {
		return nil, err
	}
	return ops.Lookup(ctx, name)
}

func hasChildLocked(ctx context.Context, parent unixfs.FSCursor, name string) (bool, error) {
	child, err := lookupChildLocked(ctx, parent, name)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, err
	}
	child.Release()
	return true, nil
}

func hasUpperChildLocked(ctx context.Context, parent unixfs.FSCursor, name string) (bool, error) {
	if parent == nil {
		return false, nil
	}
	return hasChildLocked(ctx, parent, name)
}

func cursorsHaveSameType(ctx context.Context, a, b unixfs.FSCursor) (bool, error) {
	// Resolve both cursor operations to compare their node types.
	aOps, err := cursorOps(ctx, a)
	if err != nil {
		return false, err
	}
	bOps, err := cursorOps(ctx, b)
	if err != nil {
		return false, err
	}
	return sameNodeType(aOps, bOps), nil
}

func sameNodeType(a, b unixfs.FSCursorNodeType) bool {
	return a.GetIsDirectory() == b.GetIsDirectory() &&
		a.GetIsFile() == b.GetIsFile() &&
		a.GetIsSymlink() == b.GetIsSymlink()
}

func removeUpperEntryRecursiveLocked(ctx context.Context, dirOps unixfs.FSCursorOps, name string, ts time.Time) error {
	// Open the upper child and release its cursor after removal.
	child, err := dirOps.Lookup(ctx, name)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	defer child.Release()

	// Resolve the upper child and recursively remove its descendants.
	childOps, err := cursorOps(ctx, child)
	if err != nil {
		return err
	}
	if childOps.GetIsDirectory() {
		var names []string
		if err := childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
			names = append(names, ent.GetName())
			return nil
		}); err != nil {
			return err
		}
		for _, childName := range names {
			if err := removeUpperEntryRecursiveLocked(ctx, childOps, childName, ts); err != nil {
				return err
			}
			childOps, err = cursorOps(ctx, child)
			if err != nil {
				return err
			}
		}
	}
	return dirOps.Remove(ctx, []string{name}, ts)
}

func overlayDirentFrom(ent unixfs.FSCursorDirent) unixfs.FSCursorDirent {
	return &overlayDirent{
		name:      ent.GetName(),
		isDir:     ent.GetIsDirectory(),
		isFile:    ent.GetIsFile(),
		isSymlink: ent.GetIsSymlink(),
	}
}

func whiteoutName(name string) string {
	return whiteoutPrefix + name
}

func isNotExist(err error) bool {
	return errors.Is(err, unixfs_errors.ErrNotExist)
}

type overlayDirent struct {
	name      string
	isDir     bool
	isFile    bool
	isSymlink bool
}

func (d *overlayDirent) GetName() string {
	return d.name
}

func (d *overlayDirent) GetIsDirectory() bool {
	return d.isDir
}

func (d *overlayDirent) GetIsFile() bool {
	return d.isFile
}

func (d *overlayDirent) GetIsSymlink() bool {
	return d.isSymlink
}

type cursorReader struct {
	ctx  context.Context
	ops  unixfs.FSCursorOps
	off  uint64
	size uint64
}

func (r *cursorReader) Read(p []byte) (int, error) {
	// Bound the cursor read to the remaining file content and signed offset range.
	if r.off >= r.size {
		return 0, io.EOF
	}
	remaining := r.size - r.off
	if uint64(len(p)) > remaining {
		p = p[:remaining]
	}
	if r.off > math.MaxInt64 {
		return 0, errors.New("cursor offset exceeds int64 read range")
	}

	// Read the file bytes and advance the cursor reader by the returned count.
	n, err := r.ops.ReadAt(r.ctx, int64(r.off), p) //nolint:gosec // the preceding MaxInt64 check protects the signed read API.
	if n > 0 {
		r.off += uint64(n)
		if err == io.EOF {
			err = nil
		}
		return int(n), err
	}
	return int(n), err
}

// _ is a type assertion
var _ unixfs.FSCursorOps = (*OverlayFSCursorOps)(nil)
