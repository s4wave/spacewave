package bldr

import (
	"context"
	"embed"

	util_iofs "github.com/aperturerobotics/bldr/util/iofs"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// DistSources contains the sources for the web entrypoint(s).
//
//go:embed web/bldr-react/*.ts web/bldr-react/*.tsx
//go:embed web/bldr/*.ts web/bldr/*.tsx
//go:embed web/document/*.ts web/view/*.ts
//go:embed web/electron web/entrypoint web/runtime/wasm web/index.html
//go:embed web/fetch/*.ts
//go:embed web/runtime/*.ts web/runtime/sw/*.ts
//go:embed web/runtime/wasm/go-process.ts web/runtime/wasm/plugin-wasm.ts
//go:embed web/entrypoint/browser/*.ts
//go:embed web/entrypoint/deps.go web/deps.go
//go:embed web/plugin/browser/browser_srpc.pb.ts web/plugin/browser/web-plugin-browser.ts
//go:embed web/plugin/plugin_pb.ts
//go:embed plugin/plugin_pb.ts manifest/manifest_pb.ts
//go:embed devtool/deps.go devtool/web/entrypoint/web.go
//go:embed dist/deps/deps.go dist/deps/package.json dist/deps/package-lock.json
//go:embed .vscode/launch.json
//go:embed README.md tsconfig.json go.mod go.sum global.d.ts
var DistSources embed.FS

// BuildDistSourcesFSCursor builds a *fs.Cursor for the DistSources.
func BuildDistSourcesFSCursor() *unixfs_iofs.FSCursor {
	// NOTE: we assert there is no error in src-web_test.go
	ifs := util_iofs.NewWritableFS(DistSources)
	fs, _ := unixfs_iofs.NewFSCursor(ifs)
	return fs
}

// BuildDistSourcesFSHandle builds a unixfs FSHandle for the DistSources.
func BuildDistSourcesFSHandle(ctx context.Context, le *logrus.Entry) *unixfs.FSHandle {
	fsCursor := BuildDistSourcesFSCursor()
	fsh, _ := unixfs.NewFSHandle(fsCursor)
	return fsh
}
