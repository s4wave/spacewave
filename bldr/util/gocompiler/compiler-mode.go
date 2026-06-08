package gocompiler

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

// GoCompiler identifies the compiler used for a Go plugin artifact.
type GoCompiler string

const (
	// GoCompilerDefault preserves the current default policy.
	GoCompilerDefault GoCompiler = ""
	// GoCompilerGo uses the standard Go compiler.
	GoCompilerGo GoCompiler = "go"
	// GoCompilerTinyGo uses TinyGo.
	GoCompilerTinyGo GoCompiler = "tinygo"
	// GoCompilerGoScript uses GoScript.
	GoCompilerGoScript GoCompiler = "goscript"
)

// GoCompilerEnv is the devtool-wide default Go compiler.
// Explicit manifest goCompiler config still wins over this process default.
const GoCompilerEnv = "BLDR_GO_COMPILER"

// IsTinyGo returns true when this mode uses TinyGo.
func (m GoCompiler) IsTinyGo() bool {
	return m == GoCompilerTinyGo
}

// IsGoScript returns true when this mode uses GoScript.
func (m GoCompiler) IsGoScript() bool {
	return m == GoCompilerGoScript
}

// ParseGoCompiler parses the CLI/env compiler mode value.
func ParseGoCompiler(mode string) (GoCompiler, error) {
	switch GoCompiler(strings.TrimSpace(strings.ToLower(mode))) {
	case GoCompilerDefault:
		return GoCompilerDefault, nil
	case GoCompilerGo:
		return GoCompilerGo, nil
	case GoCompilerTinyGo:
		return GoCompilerTinyGo, nil
	case GoCompilerGoScript:
		return GoCompilerGoScript, nil
	default:
		return "", errors.Errorf("unknown Go compiler %q", mode)
	}
}

// GoCompilerStartupCacheEnvKeys returns env keys that affect default Go
// compiler selection.
func GoCompilerStartupCacheEnvKeys() []string {
	return []string{GoCompilerEnv}
}

// ResolveGoCompiler resolves the Go compiler choice.
func ResolveGoCompiler(
	buildPlatform bldr_platform.Platform,
	goCompiler GoCompiler,
	defaultTinygoEnabled bool,
) (GoCompiler, error) {
	if goCompiler == GoCompilerDefault {
		envMode, err := ParseGoCompiler(os.Getenv(GoCompilerEnv))
		if err != nil {
			return "", err
		}
		if envMode != GoCompilerDefault {
			goCompiler = envMode
		}
	}

	switch goCompiler {
	case GoCompilerDefault:
	case GoCompilerGo:
		return GoCompilerGo, nil
	case GoCompilerTinyGo:
		if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
			return "", errors.Wrap(err, "tinygo enabled")
		}
		return GoCompilerTinyGo, nil
	case GoCompilerGoScript:
		return GoCompilerGoScript, nil
	default:
		return "", errors.Errorf("unknown Go compiler %q", goCompiler)
	}

	if !defaultTinygoEnabled {
		return GoCompilerGo, nil
	}
	if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
		return "", errors.Wrap(err, "tinygo enabled")
	}
	return GoCompilerTinyGo, nil
}
