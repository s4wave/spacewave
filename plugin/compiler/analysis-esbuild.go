package plugin_compiler

import (
	"strings"

	esbuild_api "github.com/evanw/esbuild/pkg/api"
	esbuild_cli "github.com/evanw/esbuild/pkg/cli"
	shellquote "github.com/kballard/go-shellquote"
)

// EsbuildTag is the bldr:esbuild constant tag used for esbuild comments.
const EsbuildTag = "bldr:esbuild"

// EsbuildArgs are arguments parsed from a bldr:esbuild directive.
type EsbuildArgs struct {
	// BuildOpts are the esbuild build options.
	BuildOpts *esbuild_api.BuildOptions
}

// TrimEsbuildArgs trims the bldr:esbuild prefix from a string.
// Returns if the string had the prefix.
func TrimEsbuildArgs(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "//")
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), EsbuildTag) {
		value = strings.TrimSpace(value[len(EsbuildTag):])
		return value, true
	}
	return value, false
}

// ParseEsbuildArgs parses the bldr:esbuild directive.
//
// Can optionally have the bldr:esbuild prefix (it will be stripped).
func ParseEsbuildArgs(value string) (*EsbuildArgs, error) {
	value, _ = TrimEsbuildArgs(value)
	args, err := shellquote.Split(value)
	if err != nil {
		return nil, err
	}
	buildOpts, err := esbuild_cli.ParseBuildOptions(args)
	if err != nil {
		return nil, err
	}
	return &EsbuildArgs{BuildOpts: &buildOpts}, nil
}
