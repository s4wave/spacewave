//go:build !js

package bldr_cli_compiler

import (
	"context"
	"go/types"
	"path"

	"github.com/pkg/errors"
	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	"github.com/sirupsen/logrus"
)

// AnalyzeCliImports discovers command builder signatures for a target platform.
func AnalyzeCliImports(ctx context.Context, le *logrus.Entry, sourcePath string, cliPkgs []string, goos, goarch string) (map[string]CliImport, error) {
	cliImports := make(map[string]CliImport)
	if len(cliPkgs) != 0 {
		cliAnalysis, err := plugin_compiler_go.AnalyzePackages(
			ctx, le, sourcePath, cliPkgs, nil, goos, goarch, false,
		)
		if err != nil {
			return nil, err
		}
		loadedPkgs := cliAnalysis.GetLoadedPackages()
		for _, cliPkg := range cliPkgs {
			pkgPath := cliPkg
			if resolved, ok := cliAnalysis.GetPackagePathMappings()[cliPkg]; ok {
				pkgPath = resolved
			}
			pkg := loadedPkgs[pkgPath]
			if pkg == nil || pkg.Types == nil {
				return nil, errors.Errorf("failed to analyze cli package %s", cliPkg)
			}
			cmdObj := pkg.Types.Scope().Lookup("NewCliCommands")
			if cmdObj == nil {
				return nil, errors.Errorf("cli package %s does not export NewCliCommands", pkgPath)
			}
			sig, ok := cmdObj.Type().(*types.Signature)
			if !ok {
				return nil, errors.Errorf("cli package %s NewCliCommands is not a function", pkgPath)
			}
			takesBroker, err := cliCommandsNeedsYieldBroker(pkgPath, sig)
			if err != nil {
				return nil, err
			}
			cliImports[cliPkg] = CliImport{Alias: path.Base(cliPkg), TakesYieldBroker: takesBroker}
		}
	}

	return cliImports, nil
}
