package plugin_platform

// List of known platform IDs.
const (
	// PlatformID_NATIVE builds Go binaries in the native executable format.
	// Dist: builds a native binary with embedded assets (i.e. a .exe).
	PlatformID_NATIVE = "native"
	// PlatformID_WEB_WASM uses the Go compiler to build WebAssembly binaries.
	// Produces WebAssembly binaries with associated html/js entrypoint files.
	// The produced bundle is self-sufficient, using the browser storage.
	// Dist: builds a directory with index.html and other assets.
	PlatformID_WEB_WASM = "web/wasm"
	// PlatformID_WEB_WS communicates with the Go runtime over a WebSocket.
	// The produced bundle requires a WebSocket connection to the native server.
	// Usually only used for the development bundle.
	// Dist: builds a server for the web entrypoint with embedded assets.
	PlatformID_WEB_WS = "web/ws"
)
