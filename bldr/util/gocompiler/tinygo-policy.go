package gocompiler

import (
	"github.com/aperturerobotics/util/enabled"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

// DefaultTinyGoEnabled returns true for release browser WebAssembly builds.
func DefaultTinyGoEnabled(buildPlatform bldr_platform.Platform, isRelease bool) bool {
	if !isRelease || !bldr_platform.IsWebPlatform(buildPlatform) {
		return false
	}
	target, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform)
	return err == nil && target == "wasm"
}

// ResolveTinyGoEnabled resolves TinyGo policy and validates supported platforms.
func ResolveTinyGoEnabled(
	buildPlatform bldr_platform.Platform,
	enableTinygoOpt enabled.Enabled,
	defaultEnabled bool,
) (bool, error) {
	mode, err := ResolveGoPluginCompilerMode(
		buildPlatform,
		resolveLegacyTinyGoMode(enableTinygoOpt),
		defaultEnabled,
	)
	if err != nil {
		return false, err
	}
	return mode.IsTinyGo(), nil
}

func resolveLegacyTinyGoMode(enableTinygoOpt enabled.Enabled) GoPluginCompilerMode {
	switch enableTinygoOpt {
	case enabled.Enabled_ENABLE:
		return GoPluginCompilerModeTinyGo
	case enabled.Enabled_DISABLE:
		return GoPluginCompilerModeGo
	default:
		return GoPluginCompilerModeDefault
	}
}
