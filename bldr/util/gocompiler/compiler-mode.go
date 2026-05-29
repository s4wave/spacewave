package gocompiler

import (
	"os"
	"strings"

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

// GoPluginCompilerModeEnv is the devtool-wide default Go plugin compiler mode.
// Explicit manifest compilerMode config still wins over this process default.
const GoPluginCompilerModeEnv = "BLDR_GO_PLUGIN_COMPILER_MODE"

// IsTinyGo returns true when this mode uses TinyGo.
func (m GoPluginCompilerMode) IsTinyGo() bool {
	return m == GoPluginCompilerModeTinyGo
}

// IsGoScript returns true when this mode uses GoScript.
func (m GoPluginCompilerMode) IsGoScript() bool {
	return m == GoPluginCompilerModeGoScript
}

// ParseGoPluginCompilerMode parses the CLI/env compiler mode value.
func ParseGoPluginCompilerMode(mode string) (GoPluginCompilerMode, error) {
	switch GoPluginCompilerMode(strings.TrimSpace(strings.ToLower(mode))) {
	case GoPluginCompilerModeDefault:
		return GoPluginCompilerModeDefault, nil
	case GoPluginCompilerModeGo:
		return GoPluginCompilerModeGo, nil
	case GoPluginCompilerModeTinyGo:
		return GoPluginCompilerModeTinyGo, nil
	case GoPluginCompilerModeGoScript:
		return GoPluginCompilerModeGoScript, nil
	default:
		return "", errors.Errorf("unknown Go plugin compiler mode %q", mode)
	}
}

// GoPluginCompilerModeStartupCacheEnvKeys returns env keys that affect default
// Go plugin compiler-mode selection.
func GoPluginCompilerModeStartupCacheEnvKeys() []string {
	return []string{GoPluginCompilerModeEnv}
}

// ResolveGoPluginCompilerMode resolves the Go plugin compiler choice.
func ResolveGoPluginCompilerMode(
	buildPlatform bldr_platform.Platform,
	compilerMode GoPluginCompilerMode,
	defaultTinygoEnabled bool,
) (GoPluginCompilerMode, error) {
	if compilerMode == GoPluginCompilerModeDefault {
		envMode, err := ParseGoPluginCompilerMode(os.Getenv(GoPluginCompilerModeEnv))
		if err != nil {
			return "", err
		}
		if envMode != GoPluginCompilerModeDefault {
			compilerMode = envMode
		}
	}

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
