package plugin_platform

// List of known platform IDs.
const (
	// PlatformID_GO_HOST uses the Go compiler to detect the host architecture.
	// Produces Go binaries in the native executable format for the build machine.
	PlatformID_GO_HOST = "go/host"
	// PlatformID_GO_WASM_WEB uses the Go compiler to build WebAssembly binaries.
	// Produces WebAssembly binaries with associated html/js entrypoint files.
	PlatformID_GO_WASM_WEB = "go/wasm/web"
	// PlatformID_GO_WS_WEB communicates with the Go runtime over a WebSocket.
	PlatformID_GO_WS_WEB = "go/ws/web"
)
