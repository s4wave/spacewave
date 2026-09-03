package gocompiler

import (
	"context"
	"os"
	"strings"

	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

// GoCompiler identifies the compiler used for a Go plugin artifact.
type GoCompiler string

const (
	// GoCompilerDefault selects GoScript for browser builds and standard Go elsewhere.
	GoCompilerDefault GoCompiler = ""
	// GoCompilerGo uses the standard Go compiler.
	GoCompilerGo GoCompiler = "go"
	// GoCompilerTinyGo uses TinyGo.
	GoCompilerTinyGo GoCompiler = "tinygo"
	// GoCompilerGoScript uses GoScript.
	GoCompilerGoScript GoCompiler = "goscript"
)

// GoCompilerEnv overrides the default compiler selection.
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

type goCompilerContextKey struct{}

// WithGoCompiler returns a context that selects mode for builds in that invocation.
func WithGoCompiler(ctx context.Context, mode GoCompiler) context.Context {
	return context.WithValue(ctx, goCompilerContextKey{}, mode)
}

func goCompilerFromContext(ctx context.Context) GoCompiler {
	mode, _ := ctx.Value(goCompilerContextKey{}).(GoCompiler)
	return mode
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
	return ResolveGoCompilerContext(context.Background(), buildPlatform, goCompiler, defaultTinygoEnabled)
}

// ResolveGoCompilerContext resolves the Go compiler choice for one invocation.
func ResolveGoCompilerContext(
	ctx context.Context,
	buildPlatform bldr_platform.Platform,
	goCompiler GoCompiler,
	defaultTinygoEnabled bool,
) (GoCompiler, error) {
	if goCompiler == GoCompilerDefault {
		envMode := goCompilerFromContext(ctx)
		var err error
		if envMode == GoCompilerDefault {
			envMode, err = ParseGoCompiler(os.Getenv(GoCompilerEnv))
		}
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

	if bldr_platform.IsWebPlatform(buildPlatform) {
		return GoCompilerGoScript, nil
	}

	if !defaultTinygoEnabled {
		return GoCompilerGo, nil
	}
	if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
		return "", errors.Wrap(err, "tinygo enabled")
	}
	return GoCompilerTinyGo, nil
}
