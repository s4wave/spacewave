package bldr

import (
	"context"
	"embed"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// WebSources contains the sources for the web entrypoint(s).
// Excludes the .go files, includes .tsx, .ts, .html only.
//
//go:embed web/bldr-react/*.tsx
//go:embed web/bldr/*.ts web/bldr/*.tsx
//go:embed web/document/*.ts web/document/view/*.ts
//go:embed web/electron web/entrypoint web/index.html
//go:embed web/fetch/*.ts web/leader/*.ts
//go:embed web/runtime/*.ts web/runtime/sw/*.ts
//go:embed package.json tsconfig.json go.mod go.sum
var WebSources embed.FS

// BuildWebSourcesFSCursor builds a *fs.Cursor for the WebSources.
func BuildWebSourcesFSCursor() *unixfs_iofs.FSCursor {
	// NOTE: we assert there is no error in src-web_test.go
	fs, _ := unixfs_iofs.NewFSCursor(WebSources)
	return fs
}

// BuildWebSourcesFS builds a unixfs FS for the WebSources.
func BuildWebSourcesFS(ctx context.Context, le *logrus.Entry) *unixfs.FS {
	fsCursor := BuildWebSourcesFSCursor()
	return unixfs.NewFS(ctx, le, fsCursor, nil)
}

// BuildWebSourcesFSHandle builds a unixfs FSHandle for the WebSources.
func BuildWebSourcesFSHandle(ctx context.Context, le *logrus.Entry) *unixfs.FSHandle {
	fs := BuildWebSourcesFS(ctx, le)
	rootRef, _ := fs.AddRootReference(ctx)
	return rootRef
}
