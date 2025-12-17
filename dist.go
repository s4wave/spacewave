package bldr

import (
	"context"
	"embed"

	util_iofs "github.com/aperturerobotics/bldr/util/iofs"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// DistSources contains the sources for the web entrypoint(s) and sdk(s).
//
// These files must be checked out to .bldr/src so TypeScript + IDEs can see them.
//
//go:embed web/bldr-react/*.ts web/bldr-react/*.tsx
//go:embed web/bldr/*.ts web/bldr/*.tsx
//go:embed web/document/*.ts web/view/*.ts web/view/handler/*.ts
//go:embed web/electron web/entrypoint web/entrypoint/index/index.html
//go:embed web/fetch/*.ts
//go:embed web/runtime/*.ts web/runtime/sw/*.ts
//go:embed web/runtime/wasm
//go:embed web/runtime/wasm/go-process.ts web/runtime/wasm/plugin-wasm.ts
//go:embed web/runtime/wasm/fetch-decompress.ts web/runtime/wasm/node-stubs.js
//go:embed web/entrypoint/browser/*.ts
//go:embed web/entrypoint/deps.go web/deps.go
//go:embed web/plugin/browser/browser_srpc.pb.ts web/plugin/browser/web-plugin-browser.ts
//go:embed web/plugin/electron/electron.pb.ts
//go:embed web/plugin/plugin.pb.ts web/plugin/plugin_srpc.pb.ts
//go:embed plugin/plugin.pb.ts plugin/plugin_srpc.pb.ts
//go:embed manifest/manifest.pb.ts manifest/manifest_srpc.pb.ts
//go:embed devtool/deps.go devtool/web/entrypoint/web.go
//go:embed dist/deps/deps.go dist/deps/package.json bun.lock
//go:embed web/bundler/bundler.pb.ts
//go:embed web/bundler/vite/build.ts web/bundler/vite/run-build.ts
//go:embed web/bundler/vite/vite.ts web/bundler/vite/plugin.ts
//go:embed web/bundler/vite/vite.pb.ts web/bundler/vite/vite_srpc.pb.ts
//go:embed web/bundler/vite/vite-base.config.ts web/bundler/vite/go-ts-resolver.ts
//go:embed util/pipesock/pipesock.ts
//go:embed plugin/compiler/js/entrypoint.ts
//go:embed sdk/plugin.ts sdk/defer.ts sdk/impl/backend-api.ts
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
