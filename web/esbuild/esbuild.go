package bldr_esbuild

import (
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
