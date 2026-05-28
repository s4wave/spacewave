package gocompiler

import (
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

// GoPluginCompilerMode identifies the compiler used for a Go plugin artifact.
type GoPluginCompilerMode string

const (
	// GoPluginCompilerModeDefault preserves the current default policy.
	GoPluginCompilerModeDefault GoPluginCompilerMode = ""
	// GoPluginCompilerModeGo uses the standard Go compiler.
	GoPluginCompilerModeGo GoPluginCompilerMode = "go"
	// GoPluginCompilerModeTinyGo uses TinyGo.
	GoPluginCompilerModeTinyGo GoPluginCompilerMode = "tinygo"
	// GoPluginCompilerModeGoScript uses GoScript.
	GoPluginCompilerModeGoScript GoPluginCompilerMode = "goscript"
)

// IsTinyGo returns true when this mode uses TinyGo.
func (m GoPluginCompilerMode) IsTinyGo() bool {
	return m == GoPluginCompilerModeTinyGo
}

// IsGoScript returns true when this mode uses GoScript.
func (m GoPluginCompilerMode) IsGoScript() bool {
	return m == GoPluginCompilerModeGoScript
}

// ResolveGoPluginCompilerMode resolves the Go plugin compiler choice.
func ResolveGoPluginCompilerMode(
	buildPlatform bldr_platform.Platform,
	compilerMode GoPluginCompilerMode,
	defaultTinygoEnabled bool,
) (GoPluginCompilerMode, error) {
	switch compilerMode {
	case GoPluginCompilerModeDefault:
	case GoPluginCompilerModeGo:
		return GoPluginCompilerModeGo, nil
	case GoPluginCompilerModeTinyGo:
		if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
			return "", errors.Wrap(err, "tinygo enabled")
		}
		return GoPluginCompilerModeTinyGo, nil
	case GoPluginCompilerModeGoScript:
		return GoPluginCompilerModeGoScript, nil
	default:
		return "", errors.Errorf("unknown Go plugin compiler mode %q", compilerMode)
	}

	if !defaultTinygoEnabled {
		return GoPluginCompilerModeGo, nil
	}
	if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
		return "", errors.Wrap(err, "tinygo enabled")
	}
	return GoPluginCompilerModeTinyGo, nil
}
