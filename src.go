package bldr

import (
	"context"
	"embed"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// MetaSources contains docs, license files, etc.
//
//go:embed LICENSE
var MetaSources embed.FS

// GetLicense returns the contents of the LICENSE file.
func GetLicense() string {
	data, _ := MetaSources.ReadFile("LICENSE")
	return string(data)
}

// BuildMetaSourcesFSCursor builds a *fs.Cursor for the MetaSources.
func BuildMetaSourcesFSCursor() *unixfs_iofs.FSCursor {
	// NOTE: we assert there is no error in assets_test.go
	fs, _ := unixfs_iofs.NewFSCursor(MetaSources)
	return fs
}

// BuildMetaSourcesFS builds a unixfs FS for the MetaSources.
func BuildMetaSourcesFS(ctx context.Context, le *logrus.Entry) *unixfs.FS {
	fsCursor := BuildMetaSourcesFSCursor()
	return unixfs.NewFS(ctx, le, fsCursor, nil)
}

// BuildMetaSourcesFSHandle builds a unixfs FSHandle for the MetaSources.
func BuildMetaSourcesFSHandle(ctx context.Context, le *logrus.Entry) *unixfs.FSHandle {
	fs := BuildMetaSourcesFS(ctx, le)
	rootRef, _ := fs.AddRootReference(ctx)
	return rootRef
}
