package unixfs_http

import (
	"context"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
)

// FileSystem implements the HTTP filesystem.
type FileSystem struct {
	http.FileSystem
	// ctx is the context to use for requests
	ctx context.Context
	// fs is the filesystem handle
	fs *unixfs.FSHandle
	// prefix is the filesystem prefix for HTTP
	prefix string
}

// NewFileSystem constructs the FileSystem from a UnixFS FSHandle.
//
// Prefix is a path prefix to prepend to file paths for HTTP.
// The prefix is trimmed from the paths when opening files.
func NewFileSystem(ctx context.Context, fsh *unixfs.FSHandle, prefix string) (*FileSystem, error) {
	if len(prefix) != 0 {
		prefix = path.Clean(prefix)
	}
	prefix = strings.TrimPrefix(prefix, "/")

	var iofs fs.FS = unixfs_iofs.NewFS(ctx, fsh)
	if prefix != "" && prefix != "." {
		var err error
		iofs, err = fs.Sub(iofs, prefix)
		if err != nil {
			return nil, err
		}
	}

	return &FileSystem{
		FileSystem: http.FS(iofs),
		ctx:        ctx,
		fs:         fsh,
		prefix:     prefix,
	}, nil
}

// GetPrefix returns the prefix that was used to build the FileSystem.
func (f *FileSystem) GetPrefix() string {
	return f.prefix
}

// CheckReleased checks if the FSHandle is released.
func (f *FileSystem) CheckReleased() bool {
	return f.fs.CheckReleased()
}

// Release releases the HTTP filesystem handle.
func (f *FileSystem) Release() {
	f.fs.Release()
}

// _ is a type assertion
var _ http.FileSystem = ((*FileSystem)(nil))
