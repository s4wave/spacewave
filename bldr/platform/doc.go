// Package bldr_platform names the platforms a Manifest can be built for and the
// deployment targets that expand to them.
//
// A platform ID names what a host can run: desktop/GOOS/GOARCH for a native
// executable, web/js/wasm for a browser WebAssembly worker, js for a JavaScript
// module, and cloudflare-workers. A target names where a build is deployed and
// expands to an ordered list of platform IDs, most preferred first: browser
// prefers web/js/wasm and then js, and desktop prefers the current
// desktop/GOOS/GOARCH and then js. The scheduler uses the same order to choose
// between Manifests of equal revision.
package bldr_platform
