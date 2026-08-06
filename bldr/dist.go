package bldr

import (
	"context"
	"embed"

	util_iofs "github.com/s4wave/spacewave/bldr/util/iofs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// DistSources contains the sources for the web entrypoint(s) and sdk(s).
// Includes entrypoints, runtime workers, shared workers, and SDK modules.
// These files must be checked out to .bldr/src so TypeScript + IDEs can see them.
//
// TODO: go.mod go.sum bun.lock tsconfig.json ?
//
//go:embed web/bldr-react/*.ts web/bldr-react/*.tsx
//go:embed web/bldr/*.ts web/bldr/*.tsx
//go:embed web/devtool-status/*.tsx web/devtool-status/*.css
//go:embed web/wasi-shim/*.ts
//go:embed web/document/*.ts web/view/*.ts web/view/handler/*.ts
//go:embed web/electron web/entrypoint web/entrypoint/index/index.html
//go:embed web/fetch/*.ts
//go:embed web/runtime/*.ts web/runtime/sw/*.ts
//go:embed web/saucer/*.ts
//go:embed web/runtime/goscript/*.ts
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
//go:embed devtool/status/status.pb.ts devtool/status/status_srpc.pb.ts
//go:embed devtool/deps.go devtool/web/entrypoint/web.go devtool/web/entrypoint/startup-trace_js.go
//go:embed dist/deps/deps.go dist/deps/package.json dist/deps/bun.lock
//go:embed web/bundler/bundler.pb.ts
//go:embed web/bundler/rolldown/run-build.mjs web/bundler/rolldown/run-build.ts web/bundler/rolldown/rolldown.pb.ts
//go:embed web/bundler/vite/build.ts web/bundler/vite/run-build.ts
//go:embed web/bundler/vite/vite.ts web/bundler/vite/plugin.ts web/bundler/vite/module-preload.ts web/bundler/vite/output-naming.ts web/bundler/vite/web-pkg-naming.ts
//go:embed web/bundler/vite/vite.pb.ts web/bundler/vite/vite_srpc.pb.ts
//go:embed web/bundler/vite/vite-base.config.ts web/bundler/vite/go-ts-resolver.ts
//go:embed plugin/compiler/js/entrypoint.ts
//go:embed resource/resource.pb.ts resource/resource_srpc.pb.ts
//go:embed resource/state/state.pb.ts resource/state/state_srpc.pb.ts
//go:embed sdk/*.ts sdk/impl/*.ts
//go:embed sdk/resource/client.ts sdk/resource/resource.ts sdk/resource/index.ts sdk/resource/unix-client.ts
//go:embed sdk/resource/resource.pb.ts sdk/resource/resource_srpc.pb.ts
//go:embed sdk/resource/server/server.ts sdk/resource/server/tracked-client.ts sdk/resource/server/attached-resource.ts
//go:embed sdk/resource/server/tracked-resource.ts sdk/resource/server/context.ts
//go:embed sdk/resource/server/mux.ts sdk/resource/server/index.ts
//go:embed sdk/state/*.ts
//go:embed sdk/plugin/host/*.ts
//go:embed sdk/hooks/*.tsx
//go:embed sdk/hooks/index.ts sdk/hooks/useMappedResource.ts sdk/hooks/useStreamingResource.ts
//go:embed sdk/hooks/resourceTransitionState.ts
//go:embed README.md global.d.ts
var DistSources embed.FS

// BuildDistSourcesFSCursor builds a *fs.Cursor for the DistSources.
func BuildDistSourcesFSCursor() *unixfs_iofs.FSCursor {
	ifs := util_iofs.NewWritableFS(DistSources)

	// NOTE: we assert there is no error in src-web_test.go
	fs, _ := unixfs_iofs.NewFSCursor(ifs)
	return fs
}

// BuildDistSourcesFSHandle builds a unixfs FSHandle for the DistSources.
func BuildDistSourcesFSHandle(ctx context.Context, le *logrus.Entry) *unixfs.FSHandle {
	fsCursor := BuildDistSourcesFSCursor()
	fsh, _ := unixfs.NewFSHandle(fsCursor)
	return fsh
}
