package bldr_web

import (
	"embed"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
)

// WebSources contains the sources for the web entrypoint(s).
// Excludes the .go files, includes .tsx, .ts, .html only.
//
//go:embed bldr-react/*.tsx
//go:embed bldr/*.ts bldr/*.tsx
//go:embed document/*.ts document/view/*.ts
//go:embed electron entrypoint index.html
//go:embed fetch/*.ts leader/*.ts
//go:embed runtime/*.ts runtime/sw/*.ts
var WebSources embed.FS

// BuildWebSourcesFSCursor builds a *fs.Cursor for the WebSources.
func BuildWebSourcesFSCursor() *unixfs_iofs.FSCursor {
	// NOTE: we assert there is no error in assets_test.go
	fs, _ := unixfs_iofs.NewFSCursor(WebSources)
	return fs
}
