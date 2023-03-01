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
	// DistPlatformID_WEB uses the Go compiler to build a WebAssembly and JavaScript bundle.
	// Builds a directory with index.html and other assets.
	DistPlatformID_WEB = "web"
)

// GetPluginPlatformID returns the plugin platform ID for the dist platform ID.
func GetPluginPlatformID(distPlatformID string) (string, error) {
	switch distPlatformID {
	case DistPlatformID_NATIVE:
		return plugin_platform.PlatformID_NATIVE, nil
	case DistPlatformID_WEB:
		return plugin_platform.PlatformID_WEB_WASM, nil
	default:
		return "", errors.Errorf("unknown dist platform id: %s", distPlatformID)
	}
}
