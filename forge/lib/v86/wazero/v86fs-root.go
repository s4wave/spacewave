package v86_wazero

import (
	"os"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_overlay "github.com/s4wave/spacewave/db/unixfs/overlay"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

// OpenV86Root builds a v86fs Server that serves rootfsTar as the guest root.
// The returned release func drops the root FSHandle and must run after the
// guest stops using the server.
func OpenV86Root(mode RootMode, rootfsTar string) (*unixfs_v86fs.Server, func(), error) {
	mode, err := normalizeRootMode(mode)
	if err != nil {
		return nil, nil, err
	}

	switch mode.Mode {
	case rootModeReadonly:
		cursor, err := openV86RootLower(rootfsTar)
		if err != nil {
			return nil, nil, err
		}
		return openV86RootServer(cursor)
	case rootModeRAM:
		lower, err := openV86RootLower(rootfsTar)
		if err != nil {
			return nil, nil, err
		}
		upper := unixfs_billy.NewBillyFSCursor(memfs.New(), "")
		return openV86RootServer(unixfs_overlay.NewOverlayFSCursor(lower, upper))
	case rootModeDisk:
		if err := os.MkdirAll(mode.Arg, 0o755); err != nil {
			return nil, nil, errors.Wrap(err, "create disk root upper")
		}
		lower, err := openV86RootLower(rootfsTar)
		if err != nil {
			return nil, nil, err
		}
		upper := unixfs_billy.NewBillyFSCursor(osfs.New(mode.Arg), "")
		return openV86RootServer(unixfs_overlay.NewOverlayFSCursor(lower, upper))
	case rootModeVolume:
		// TODO: back volume root mode with db/unixfs/block.
		return nil, nil, errors.Errorf("root-mode volume not yet implemented")
	case rootModeDaemon:
		// TODO: back daemon root mode with db/unixfs/world.
		return nil, nil, errors.Errorf("root-mode daemon not yet implemented")
	default:
		return nil, nil, errors.Errorf("unknown root-mode %q, want %s", mode.Mode, rootModeAllowed)
	}
}

// openV86RootLower parses the rootfs tar into a read-only lower cursor.
func openV86RootLower(rootfsTar string) (unixfs.FSCursor, error) {
	f, err := os.Open(rootfsTar)
	if err != nil {
		return nil, errors.Wrap(err, "open rootfs tar")
	}
	defer f.Close()
	cursor, err := unixfs_tar.NewTarFSCursorFromReader(f)
	if err != nil {
		return nil, errors.Wrap(err, "parse rootfs tar")
	}
	return cursor, nil
}

// openV86RootServer wraps a root cursor in a v86fs server serving /.
func openV86RootServer(cursor unixfs.FSCursor) (*unixfs_v86fs.Server, func(), error) {
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return nil, nil, errors.Wrap(err, "build rootfs handle")
	}
	server := unixfs_v86fs.NewServer(nil, nil)
	server.AddMount("", "/", handle)
	return server, handle.Release, nil
}
