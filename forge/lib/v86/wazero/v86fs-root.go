package v86_wazero

import (
	"context"
	"os"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/pkg/errors"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_overlay "github.com/s4wave/spacewave/db/unixfs/overlay"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
)

// OpenV86Root builds a v86fs Server that serves the image's rootfs as the
// guest root. A local archive is parsed into memory. A CDN rootfs object is
// read lazily from the published world. The returned release func drops the
// root and any CDN mount and must run after the guest stops using the server.
func OpenV86Root(ctx context.Context, mode RootMode, assets *AssetSet) (*unixfs_v86fs.Server, func(), error) {
	// Validate the mode before allocating either layer.
	mode, err := normalizeRootMode(mode)
	if err != nil {
		return nil, nil, err
	}

	// Build the upper layer before opening the lower so a bad mode leaks nothing.
	var upper unixfs.FSCursor
	switch mode.Mode {
	case rootModeReadonly:
	case rootModeRAM:
		upper = unixfs_billy.NewBillyFSCursor(memfs.New(), "")
	case rootModeDisk:
		if err := os.MkdirAll(mode.Arg, 0o755); err != nil {
			return nil, nil, errors.Wrap(err, "create disk root upper")
		}
		upper = unixfs_billy.NewBillyFSCursor(osfs.New(mode.Arg), "")
	case rootModeVolume:
		// TODO: back volume root mode with db/unixfs/block.
		return nil, nil, errors.Errorf("root-mode volume not yet implemented")
	case rootModeDaemon:
		// TODO: back daemon root mode with db/unixfs/world.
		return nil, nil, errors.Errorf("root-mode daemon not yet implemented")
	default:
		return nil, nil, errors.Errorf("unknown root-mode %q, want %s", mode.Mode, rootModeAllowed)
	}

	// Open the image filesystem as the read-only lower layer.
	lower, releaseLower, err := openV86RootLower(ctx, assets)
	if err != nil {
		if upper != nil {
			upper.Release()
		}
		return nil, nil, err
	}
	root := lower
	if upper != nil {
		root = unixfs_overlay.NewOverlayFSCursor(lower, upper)
	}

	// Serve the layered root and release it before the lower layer's mount.
	server, releaseRoot, err := openV86RootServer(root)
	if err != nil {
		releaseLower()
		return nil, nil, err
	}
	return server, func() {
		releaseRoot()
		releaseLower()
	}, nil
}

// openV86RootLower opens the image rootfs as a read-only cursor. The release
// func drops the CDN world mount, if any, after the cursor is released.
func openV86RootLower(ctx context.Context, assets *AssetSet) (unixfs.FSCursor, func(), error) {
	// Prefer the local archive, which needs no CDN mount.
	if assets.RootfsTar != "" {
		cursor, err := openRootfsTar(assets.RootfsTar)
		if err != nil {
			return nil, nil, err
		}
		return cursor, func() {}, nil
	}
	if assets.RootfsObjectKey == "" {
		return nil, nil, errors.New("v86 image has neither a rootfs tar nor a rootfs object")
	}

	// Keep the published world mounted for the lifetime of the lazy cursor.
	opts := assets.cdn.withDefaults()
	ws, err := cdn_sharedobject.MountWorld(ctx, opts.Le, nil, opts.CdnBaseURL, opts.CdnSpaceID)
	if err != nil {
		return nil, nil, err
	}
	releaseWorld := ws.Release
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, assets.RootfsObjectKey)
	if err != nil {
		releaseWorld()
		return nil, nil, errors.Wrapf(err, "lookup rootfs object %q", assets.RootfsObjectKey)
	}
	cursor := unixfs_world.NewFSCursorWithContext(ctx, opts.Le, ws, assets.RootfsObjectKey, fsType, nil, false)
	return cursor, releaseWorld, nil
}

// openRootfsTar parses a rootfs tar into a read-only cursor.
func openRootfsTar(rootfsTar string) (unixfs.FSCursor, error) {
	// Open the guest rootfs archive and close it after parsing.
	f, err := os.Open(rootfsTar)
	if err != nil {
		return nil, errors.Wrap(err, "open rootfs tar")
	}
	defer f.Close()

	// Parse the archive into its read-only filesystem cursor.
	cursor, err := unixfs_tar.NewTarFSCursorFromReader(f)
	if err != nil {
		return nil, errors.Wrap(err, "parse rootfs tar")
	}
	return cursor, nil
}

// openV86RootServer wraps a root cursor in a v86fs server serving /.
func openV86RootServer(cursor unixfs.FSCursor) (*unixfs_v86fs.Server, func(), error) {
	// Transfer the guest root cursor into a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return nil, nil, errors.Wrap(err, "build rootfs handle")
	}

	// Mount the guest root handle at the v86fs server root.
	server := unixfs_v86fs.NewServer(nil, nil)
	server.AddMount("", "/", handle)

	return server, handle.Release, nil
}
