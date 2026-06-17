package bifrost_http

import "net/http"

// maybeServeDecodedBrotli is a no-op in the js/wasm browser build. Browsers
// always send Accept-Encoding: br, so the non-js fallback that decodes a ".br"
// asset for a non-brotli client is unreachable here. Returning false keeps the
// brotli decoder and its dictionary out of the runtime bundle; the request
// falls through to the normal file server.
func maybeServeDecodedBrotli(_ http.ResponseWriter, _ *http.Request, _ http.FileSystem) bool {
	return false
}
