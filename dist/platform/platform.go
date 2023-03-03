package dist_platform

import (
	plugin_platform "github.com/aperturerobotics/bldr/plugin/platform"
	"github.com/pkg/errors"
)

// DistPlatformID identifies the platform used as an entrypoint.
const (
	// DistPlatformID_NATIVE builds Go binaries in the native executable format.
	// Builds a native binary with embedded assets (i.e. a .exe).
	DistPlatformID_NATIVE = "native"
	// DistPlatformID_NATIVE_DEV is set when running the devtool natively (desktop/electron).
	// All instances of the app share the same bus within the devtool.
	DistPlatformID_NATIVE_DEV = "native/dev"
	// DistPlatformID_WEB uses the Go compiler to build a WebAssembly and JavaScript bundle.
	// Builds a directory with index.html and other assets.
	DistPlatformID_WEB = "web"
	// DistPlatformID_WEB_DEV is set when running the devtool web server.
	// All instances of the app share the same bus via a WebSocket.
	DistPlatformID_WEB_DEV = "web/dev"
)

// GetPluginPlatformID returns the plugin platform ID for the dist platform ID.
func GetPluginPlatformID(distPlatformID string) (string, error) {
	switch distPlatformID {
	case DistPlatformID_NATIVE_DEV:
		// The plugins are loaded natively in the devtool.
		return plugin_platform.PlatformID_NATIVE, nil
	case DistPlatformID_NATIVE:
		// The plugins are loaded natively in the distribution entrypoint.
		return plugin_platform.PlatformID_NATIVE, nil
	case DistPlatformID_WEB_DEV:
		// The plugins are loaded natively in the devtool.
		return plugin_platform.PlatformID_NATIVE, nil
	case DistPlatformID_WEB:
		// The plugins are loaded as wasm modules in the browser.
		return plugin_platform.PlatformID_WEB_WASM, nil
	default:
		return "", errors.Errorf("unknown dist platform id: %s", distPlatformID)
	}
}
