//go:build !js

package bldr_cli_compiler

import (
	"context"
	"path"

	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	"github.com/sirupsen/logrus"
)

// AnalyzeCliImports validates the command builders of cliPkgs for a target platform.
func AnalyzeCliImports(ctx context.Context, le *logrus.Entry, sourcePath string, cliPkgs []string, goos, goarch string) (map[string]CliImport, error) {
	// Avoid loading the helper module when no CLI packages were requested.
	cliImports := make(map[string]CliImport)
	if len(cliPkgs) == 0 {
		return cliImports, nil
	}

	// Discover constructors using the same target selection as generated factories.
	analysis, err := plugin_compiler_go.AnalyzePackages(
		ctx, le, sourcePath, cliPkgs, nil, goos, goarch, false,
	)
	if err != nil {
		return nil, err
	}

	// Validate constructors before projecting package aliases for CLI generation.
	if err := analysis.ValidateCliImports(cliPkgs); err != nil {
		return nil, err
	}
	for _, cliPkg := range cliPkgs {
		cliImports[cliPkg] = CliImport{Alias: path.Base(cliPkg)}
	}
	return cliImports, nil
}
