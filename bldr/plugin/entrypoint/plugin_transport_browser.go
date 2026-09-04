//go:build js && !bldr_cloudflare

package plugin_entrypoint

import (
	web_runtime_wasm "github.com/s4wave/spacewave/bldr/web/runtime/wasm"
)

// newPluginTransport resolves the plugin transport for the current runtime.
// For browser js/wasm builds this obtains the MessagePort-based IO from the
// browser runtime globals (see plugin-wasm.ts). A Cloudflare Workers build
// (//go:build js && bldr_cloudflare) will define the same function, returning
// a transport over the Worker frame boundary.
func newPluginTransport() (pluginTransport, error) {
	return web_runtime_wasm.GlobalWasmPluginIo()
}
