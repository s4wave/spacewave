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
// Includes entrypoints, runtime workers, shared package import bindings, and SDK modules.
// These files must be checked out to .bldr/src so TypeScript + IDEs can see them.
//
// TODO: go.mod go.sum bun.lock tsconfig.json ?
//
//go:embed web/bldr-react/callback.ts web/bldr-react/index.ts
//go:embed web/bldr-react/merge-refs.ts web/bldr-react/react-component.ts
//go:embed web/bldr-react/web-view-module-loader.ts web/bldr-react/*.tsx
//go:embed web/bldr/abort.ts web/bldr/binary.ts web/bldr/boot-downloads.ts
//go:embed web/bldr/boot-progress.ts web/bldr/browser-release-state.ts
//go:embed web/bldr/browser-release-update.ts web/bldr/callback-promise.ts
//go:embed web/bldr/channel-packet-stream.ts web/bldr/constants.ts
//go:embed web/bldr/cross-tab-broker.ts web/bldr/cross-tab-manager.ts
//go:embed web/bldr/dedicated-worker-host-owner.ts web/bldr/format.ts
//go:embed web/bldr/frontend.ts web/bldr/index.ts web/bldr/it-state.ts
//go:embed web/bldr/message-port-bridge.ts web/bldr/opfs-worker.ts
//go:embed web/bldr/path.ts web/bldr/plugin-entrypoint.ts
//go:embed web/bldr/plugin-transport.ts web/bldr/random-id.ts web/bldr/retry.ts
//go:embed web/bldr/sab-pair-broker.ts web/bldr/sab-ring-stream.ts
//go:embed web/bldr/service-worker-fetch-tracker.ts web/bldr/service-worker.ts
//go:embed web/bldr/shared-worker.ts web/bldr/shutdown.ts
//go:embed web/bldr/simple-event-emitter.ts web/bldr/startup-marks.ts
//go:embed web/bldr/storage-durability-owner.ts web/bldr/timeout.ts
//go:embed web/bldr/stylex-client.ts
//go:embed web/bldr/stylex-order.ts web/bldr/stylex-priorities.ts
//go:embed web/bldr/ulid.ts web/bldr/wasm-detect.ts
//go:embed web/bldr/web-document-lock.ts web/bldr/web-document-tracker.ts
//go:embed web/bldr/web-document.ts web/bldr/web-pkg-imports.ts
//go:embed web/bldr/web-runtime-client.ts web/bldr/web-runtime.ts
//go:embed web/bldr/web-view.ts web/bldr/webrtc-bridge-endpoint.ts
//go:embed web/bldr/worker-comms-detect.ts web/bldr/zero-native-webview-ipc.ts
//go:embed web/boot/codec.ts web/boot/collector.ts web/boot/report.pb.ts
//go:embed web/boot/store.ts web/boot/validator.ts web/boot/vocabulary.ts
//go:embed web/devtool-status/*.tsx web/devtool-status/*.css
//go:embed web/wasi-shim/*.ts
//go:embed web/document/*.ts web/view/*.ts web/view/handler/*.ts
//go:embed web/electron/desktop-runtime/desktop-runtime.pb.go
//go:embed web/electron/desktop-runtime/desktop-runtime.pb.ts
//go:embed web/electron/desktop-runtime/desktop-runtime.proto
//go:embed web/electron/desktop-runtime/desktop-runtime_srpc.pb.go
//go:embed web/electron/desktop-runtime/desktop-runtime_srpc.pb.ts
//go:embed web/electron/electron.ts
//go:embed web/electron/main/app-menu.ts web/electron/main/app-update.ts
//go:embed web/electron/main/app.ts
//go:embed web/electron/main/desktop-cli-install-detector.ts
//go:embed web/electron/main/desktop-cli-install-executor.ts
//go:embed web/electron/main/desktop-cli-install-node.ts
//go:embed web/electron/main/desktop-cli-install-resource.ts
//go:embed web/electron/main/desktop-cli-install-target-policy.ts
//go:embed web/electron/main/desktop-runtime.ts
//go:embed web/electron/main/desktop-tray-resource.ts
//go:embed web/electron/main/desktop-tray-runtime-projection.ts
//go:embed web/electron/main/desktop-tray.ts web/electron/main/index.ts
//go:embed web/electron/main/ipc.ts web/electron/main/preload.ts
//go:embed web/electron/main/process-stream.ts web/electron/main/protocol.ts
//go:embed web/entrypoint/FrontendStartup.tsx web/entrypoint/app-path.ts
//go:embed web/entrypoint/boot-status.ts
//go:embed web/entrypoint/browser/README.md
//go:embed web/entrypoint/browser/browser.e2e.spec.ts
//go:embed web/entrypoint/browser/browser.go
//go:embed web/entrypoint/browser/browser.pb.go
//go:embed web/entrypoint/browser/browser.pb.ts
//go:embed web/entrypoint/browser/browser.proto
//go:embed web/entrypoint/browser/browser_js.go
//go:embed web/entrypoint/browser/build/build.go
//go:embed web/entrypoint/browser/bundle/bundle-cache.go
//go:embed web/entrypoint/browser/bundle/bundle.go
//go:embed web/entrypoint/browser/bundle/internal-build.go
//go:embed web/entrypoint/browser/bundle/renderer-vite.go
//go:embed web/entrypoint/browser/bundle/tinygo-wasm-exec.go
//go:embed web/entrypoint/browser/config.go web/entrypoint/browser/factory.go
//go:embed web/entrypoint/browser/message-port-packet-stream.ts
//go:embed web/entrypoint/browser/message-port/message-channel.go
//go:embed web/entrypoint/browser/message-port/message-port.go
//go:embed web/entrypoint/browser/message-port/packet.go
//go:embed web/entrypoint/browser/playwright.config.ts
//go:embed web/entrypoint/browser/runtime-goscript.ts
//go:embed web/entrypoint/browser/runtime-opfs-bridge.ts
//go:embed web/entrypoint/browser/runtime-wasm.ts
//go:embed web/entrypoint/deps.go
//go:embed web/entrypoint/electron/bundle/branding.go
//go:embed web/entrypoint/electron/bundle/bundle.go
//go:embed web/entrypoint/electron/bundle/internal-build.go
//go:embed web/entrypoint/electron/bundle/sign.go
//go:embed web/entrypoint/entrypoint.tsx
//go:embed web/entrypoint/index/index.go web/entrypoint/index/index.html
//go:embed web/entrypoint/saucer/bundle/bundle.go
//go:embed web/entrypoint/startuptrace/startup-trace_js.go
//go:embed web/entrypoint/startuptrace/startup-trace_stub.go
//go:embed web/fetch/fetch.pb.ts web/fetch/fetch.ts web/fetch/fetch_srpc.pb.ts
//go:embed web/fetch/readablestream-to-it.ts
//go:embed web/runtime/opfs-bridge-client.ts web/runtime/plugin-worker.ts
//go:embed web/runtime/runtime.pb.ts web/runtime/runtime.ts
//go:embed web/runtime/runtime_srpc.pb.ts
//go:embed web/runtime/sw/*.ts
//go:embed web/saucer/*.ts
//go:embed web/runtime/goscript/open-stream-contract.ts
//go:embed web/runtime/goscript/plugin-goscript-cloudflare.ts
//go:embed web/runtime/goscript/plugin-goscript.ts
//go:embed web/runtime/wasm/build/build.go
//go:embed web/runtime/wasm/fetch-decompress.ts web/runtime/wasm/go-process.ts
//go:embed web/runtime/wasm/node-stubs.js web/runtime/wasm/plugin-wasm.ts
//go:embed web/runtime/wasm/pushable_js.go
//go:embed web/runtime/wasm/stream_bridge_other.go
//go:embed web/runtime/wasm/stream_bridge_tinygo.go web/runtime/wasm/wasm.go
//go:embed web/runtime/wasm/wasm.pb.go web/runtime/wasm/wasm.pb.ts
//go:embed web/runtime/wasm/wasm.proto web/runtime/wasm/wasm_js.go
//go:embed web/runtime/wasm/webrtc-bridge.ts
//go:embed web/deps.go
//go:embed web/plugin/browser/browser_srpc.pb.ts
//go:embed web/plugin/browser/web-plugin-browser.ts
//go:embed web/plugin/electron/electron.pb.ts
//go:embed web/plugin/plugin.pb.ts web/plugin/plugin_srpc.pb.ts
//go:embed desktop/control/control.pb.ts desktop/control/control_srpc.pb.ts
//go:embed desktop/update/update.pb.ts
//go:embed plugin/plugin.pb.ts plugin/plugin_srpc.pb.ts
//go:embed manifest/manifest.pb.ts manifest/manifest_srpc.pb.ts
//go:embed devtool/status/status.pb.ts devtool/status/status_srpc.pb.ts
//go:embed devtool/deps.go
//go:embed devtool/web/entrypoint/web.go
//go:embed devtool/web/entrypoint/startup-trace_js.go
//go:embed dist/deps/patches/*.patch
//go:embed dist/deps/deps.go dist/deps/package.json dist/deps/bun.lock
//go:embed web/bundler/bundler.pb.ts
//go:embed web/bundler/esbuild/esbuild.pb.ts
//go:embed web/bundler/esbuild/compiler/compiler.pb.ts
//go:embed web/bundler/vite/compiler/compiler.pb.ts web/pkg/pkg.pb.ts
//go:embed web/bundler/rolldown/run-build.mjs web/bundler/rolldown/run-build.ts
//go:embed web/bundler/rolldown/rolldown.pb.ts
//go:embed web/bundler/vite/build.ts web/bundler/vite/run-build.ts
//go:embed web/bundler/vite/stylex.ts
//go:embed web/bundler/vite/vite.ts web/bundler/vite/plugin.ts
//go:embed web/bundler/vite/module-preload.ts web/bundler/vite/output-naming.ts
//go:embed web/bundler/vite/web-pkg-naming.ts
//go:embed web/bundler/vite/vite.pb.ts web/bundler/vite/vite_srpc.pb.ts
//go:embed web/bundler/vite/development.ts
//go:embed web/bundler/vite/development-client.ts
//go:embed frontend/*.ts
//go:embed web/bundler/vite/vite-base.config.ts
//go:embed web/bundler/go-module-cache.ts
//go:embed web/bundler/vite/go-ts-resolver.ts
//go:embed plugin/compiler/js/compiler.pb.ts plugin/compiler/js/entrypoint.ts
//go:embed project/validate/validate.pb.ts
//go:embed resource/resource.pb.ts resource/resource_srpc.pb.ts resource/view.pb.ts
//go:embed resource/state/state.pb.ts resource/state/state_srpc.pb.ts
//go:embed sdk/*.ts sdk/impl/*.ts
//go:embed sdk/resource/client.ts sdk/resource/resource.ts sdk/resource/index.ts
//go:embed sdk/resource/unix-client.ts
//go:embed sdk/resource/rpc-stream.ts sdk/resource/shared-streams.ts sdk/resource/held-views.ts sdk/resource/view.pb.ts
//go:embed sdk/resource/resource.pb.ts sdk/resource/resource_srpc.pb.ts
//go:embed sdk/resource/server/server.ts sdk/resource/server/tracked-client.ts
//go:embed sdk/resource/server/attached-resource.ts
//go:embed sdk/resource/server/tracked-resource.ts
//go:embed sdk/resource/server/context.ts
//go:embed sdk/resource/server/mux.ts sdk/resource/server/index.ts
//go:embed sdk/state/*.ts
//go:embed sdk/plugin/host/*.ts
//go:embed sdk/hooks/*.tsx
//go:embed sdk/hooks/index.ts sdk/hooks/useMappedResource.ts
//go:embed sdk/hooks/useStreamingResource.ts
//go:embed sdk/hooks/resourceTransitionState.ts sdk/hooks/SharedResources.ts
//go:embed web/bldr-react/stylex-types.ts
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
