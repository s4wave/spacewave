package v86_wazero

import (
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

// OpenV86FSRoot builds a v86fs Server that serves rootfsTar as the guest root
// filesystem mounted at "/". NewTarFSCursorFromReader copies the archive into
// memory, so the file is closed before returning; the returned release func
// drops the root FSHandle and must run after the guest stops using the server.
func OpenV86FSRoot(rootfsTar string) (*unixfs_v86fs.Server, func(), error) {
	f, err := os.Open(rootfsTar)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open rootfs tar")
	}
	defer f.Close()
	cursor, err := unixfs_tar.NewTarFSCursorFromReader(f)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse rootfs tar")
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build rootfs handle")
	}
	server := unixfs_v86fs.NewServer(nil)
	server.AddMount("", "/", handle)
	return server, handle.Release, nil
}
