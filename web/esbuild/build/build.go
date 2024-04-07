package bldr_esbuild_build

import (
	"errors"

	esbuild_api "github.com/evanw/esbuild/pkg/api"
	esbuild_cli "github.com/evanw/esbuild/pkg/cli"
	shellquote "github.com/kballard/go-shellquote"
)

// ParseEsbuildFlags parsed the esbuild flags field, if set.
// Returns nil if no flags were set.
func ParseEsbuildFlags(flags []string) (*esbuild_api.BuildOptions, error) {
	var args []string
	for _, flagStr := range flags {
		flagArgs, err := shellquote.Split(flagStr)
		if err != nil {
			return nil, err
		}
		args = append(args, flagArgs...)
	}
	if len(args) == 0 {
		return nil, nil
	}

	opts, err := esbuild_cli.ParseBuildOptions(args)
	if err != nil {
		return nil, err
	}
	return &opts, nil
}

// BuildResultToErr converts a BuildResult into a single error, if any.
func BuildResultToErr(res esbuild_api.BuildResult) error {
	if len(res.Errors) == 0 {
		return nil
	}
	return errors.New(res.Errors[0].Text)
}

// ResolveResultToErr converts a BuildResult into a single error, if any.
func ResolveResultToErr(res esbuild_api.ResolveResult) error {
	if len(res.Errors) == 0 {
		return nil
	}
	return errors.New(res.Errors[0].Text)
}
