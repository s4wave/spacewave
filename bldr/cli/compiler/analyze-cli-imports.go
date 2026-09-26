//go:build !js

package bldr_cli_compiler

import (
	"context"
	"path"

	"github.com/pkg/errors"
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

	// CLI builders require exactly one parameter, including any variadic tail.
	for _, cliPkg := range cliPkgs {
		pkgPath := cliPkg
		if resolved, ok := analysis.GetPackagePathMappings()[cliPkg]; ok {
			pkgPath = resolved
		}
		pkg := analysis.GetPackages()[pkgPath]
		if pkg == nil {
			return nil, errors.Errorf("failed to analyze cli package %s", cliPkg)
		}
		constructor := pkg.CliCommands
		if constructor == nil {
			return nil, errors.Errorf("cli package %s does not export NewCliCommands", pkgPath)
		}
		if !constructor.IsFunction {
			return nil, errors.Errorf("cli package %s NewCliCommands is not a function", pkgPath)
		}
		if constructor.Parameters != 1 {
			return nil, errors.Errorf("cli package %s NewCliCommands must take only getBus", pkgPath)
		}
		cliImports[cliPkg] = CliImport{Alias: path.Base(cliPkg)}
	}
	return cliImports, nil
}
