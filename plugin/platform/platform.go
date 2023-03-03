package plugin_platform

// PlatformID identifies the platform used to load plugins.
const (
	// PlatformID_NATIVE builds Go binaries in the native executable format.
	// Usually corresponds to the process plugin host, but may also be containerized.
	// TODO: include architecture information?
	PlatformID_NATIVE = "native"
	// PlatformID_WEB_WASM uses the Go compiler to build WebAssembly binaries.
	PlatformID_WEB_WASM = "web/wasm"
)
