package unixfs_sync

import (
	"context"
	"path"

	"github.com/s4wave/spacewave/db/unixfs"
)

// SyncXattrs applies extended attributes from the source UnixFS tree
// to the files on disk at outPath. Call after Sync to preserve xattrs.
// This is a separate pass because billy.Filesystem does not support xattrs.
func SyncXattrs(
	ctx context.Context,
	outPath string,
	fsHandle *unixfs.FSHandle,
) error {
	return syncXattrsRecursive(ctx, outPath, fsHandle, "")
}

func syncXattrsRecursive(
	ctx context.Context,
	outBase string,
	handle *unixfs.FSHandle,
	relPath string,
) error {
	if handle.CheckReleased() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get xattrs for this node.
	xattrs, err := handle.GetXattrs(ctx)
	if err != nil {
		return err
	}
	if len(xattrs) > 0 {
		diskPath := outBase
		if relPath != "" {
			diskPath = path.Join(outBase, relPath)
		}
		if err := applyXattrs(diskPath, xattrs); err != nil {
			return err
		}
	}

	// If directory, recurse into children.
	fi, err := handle.GetFileInfo(ctx)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return nil
	}

	return handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		name := ent.GetName()
		childHandle, err := handle.Lookup(ctx, name)
		if err != nil {
			return nil // skip missing entries
		}
		defer childHandle.Release()
		childRel := name
		if relPath != "" {
			childRel = path.Join(relPath, name)
		}
		return syncXattrsRecursive(ctx, outBase, childHandle, childRel)
	})
}
