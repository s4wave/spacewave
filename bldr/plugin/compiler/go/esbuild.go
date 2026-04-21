//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bldr_web_bundler_esbuild "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	bldr_web_bundler_esbuild_compiler "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/compiler"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildEsbuildBundlerConfig builds the esbuild bundler controller config.
func BuildEsbuildBundlerConfig(
	bundleVars []*EsbuildBundleVarMeta,
	webPkgs []*bldr_web_bundler.WebPkgRefConfig,
	baseEsbuildFlags []string,
	codeRootPath,
	publicPath string,
) (*bldr_web_bundler_esbuild_compiler.Config, error) {
	// build list of EsbuildBundleMeta from bundleVar list
	var esbuildBundleMeta []*bldr_web_bundler_esbuild_compiler.EsbuildBundleMeta
	for _, bundleVar := range bundleVars {
		var bundleFlags []string
		var bundleEntrypoints []*bldr_web_bundler_esbuild.EsbuildBundleEntrypoint
		for _, bundleVarEntrypoint := range bundleVar.GetEntrypointVars() {
			varEsbuildFlags := bundleVarEntrypoint.GetEsbuildFlags()
			varBuildOpts, err := bldr_esbuild_build.ParseEsbuildFlags(varEsbuildFlags)
			if err != nil {
				return nil, err
			}

			entryPoints := slices.Clone(varBuildOpts.EntryPointsAdvanced)

			// convert non-advanced entrypoints to advanced
			for _, entrypointPath := range varBuildOpts.EntryPoints {
				entryPoints = append(entryPoints, esbuild_api.EntryPoint{
					InputPath: entrypointPath,
				})
			}

			// transform entrypoint paths to be relative to codeRootPath
			for i := range entryPoints {
				inputPath := entryPoints[i].InputPath
				// treat absolute paths as relative to root of project
				if filepath.IsAbs(inputPath) {
					inputPath = filepath.Join(codeRootPath, inputPath)
					inputPath, err = filepath.Rel(codeRootPath, inputPath)
					if err != nil {
						return nil, err
					}
				} else {
					// determine path relative to the code
					inputPath = filepath.ToSlash(filepath.Join(bundleVarEntrypoint.GetPkgCodePath(), filepath.Clean(inputPath)))
				}

				// double-check to make sure path is within the code root
				inputPath = filepath.Join(codeRootPath, inputPath)
				inputPath, err = filepath.Rel(codeRootPath, inputPath)
				if err != nil {
					return nil, err
				}
				if strings.HasPrefix(inputPath, "../") {
					return nil, errors.Errorf("entrypoint cannot be outside code root: %s", inputPath)
				}

				entryPoints[i].InputPath = inputPath
			}

			// restrict to a single entrypoint per variable.
			// NOTE; we could remove this restriction, but then we wouldn't know which to store in the variable.
			if len(entryPoints) != 1 {
				return nil, errors.Errorf(
					"expected single entrypoint per bldr:esbuild variable but got %v: %s.%s",
					len(entryPoints),
					bundleVarEntrypoint.PkgImportPath,
					bundleVarEntrypoint.PkgVar,
				)
			}

			// note: we enforce just one here, but use a for loop for completeness.
			bundleFlags = append(bundleFlags, varEsbuildFlags...)
			for _, entryPoint := range entryPoints {
				bundleEntrypoints = append(bundleEntrypoints, &bldr_web_bundler_esbuild.EsbuildBundleEntrypoint{
					EntrypointId: bundleVarEntrypoint.ToEsbuildEntrypointId(bundleVar.GetId()),
					InputPath:    entryPoint.InputPath,
					OutputPath:   entryPoint.OutputPath,
				})
			}
		}

		// build the bundle metadata
		bundleMeta := &bldr_web_bundler_esbuild_compiler.EsbuildBundleMeta{
			Id:           bundleVar.Id,
			Entrypoints:  bundleEntrypoints,
			PublicPath:   publicPath,
			EsbuildFlags: bundleFlags,
		}

		// add to the bundle meta list
		esbuildBundleMeta = append(esbuildBundleMeta, bundleMeta)
	}

	return &bldr_web_bundler_esbuild_compiler.Config{
		Bundles:      esbuildBundleMeta,
		WebPkgs:      webPkgs,
		EsbuildFlags: baseEsbuildFlags,
	}, nil
}

// BuildEsbuildBundleVarMeta builds the bundle metadata from the list of go variable defs.
func BuildEsbuildBundleVarMeta(
	le *logrus.Entry,
	codeRootPath string,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*EsbuildDirective),
) ([]*EsbuildBundleVarMeta, error) {
	// bundles is the map of bundle-id to bundle-def
	bundles := make(map[string]*EsbuildBundleVarMeta)
	getBundle := func(bundleID string) *EsbuildBundleVarMeta {
		bundleDef := bundles[bundleID]
		if bundleDef != nil {
			return bundleDef
		}

		bundleDef = &EsbuildBundleVarMeta{Id: bundleID}
		bundles[bundleID] = bundleDef
		return bundleDef
	}

	// for each package variable, build a bundle definition + variable
	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, errors.Errorf("failed to find ast.File for package: %s", pkgImportPath)
		}

		pkgCodePath := filepath.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
		relPkgCodePath, err := filepath.Rel(codeRootPath, pkgCodePath)
		if err != nil {
			return nil, errors.Wrap(err, "unable to determine relative path")
		}

		for pkgVar, pkgEsbuildDirective := range pkgVars {
			buildFlags := pkgEsbuildDirective.EsbuildFlags
			if len(buildFlags) == 0 {
				return nil, errors.Errorf("%s: expected at least one entrypoint", pkgImportPath+"."+pkgVar)
			}

			bundleID := pkgEsbuildDirective.BundleID
			bundleDef := getBundle(bundleID)
			bundleDef.EntrypointVars = append(bundleDef.EntrypointVars, &EsbuildEntrypointVar{
				PkgImportPath: pkgImportPath,
				PkgVar:        pkgVar,
				PkgVarType:    pkgEsbuildDirective.EsbuildVarType,
				PkgCodePath:   relPkgCodePath,
				EsbuildFlags:  buildFlags,
			})
		}
	}

	// sort entrypoint variables
	bundleVals := slices.Collect(maps.Values(bundles))
	for _, bundle := range bundleVals {
		bundle.SortEntrypointVars()
	}

	// sort by bundle id
	slices.SortFunc(bundleVals, func(a, b *EsbuildBundleVarMeta) int {
		return strings.Compare(a.GetId(), b.GetId())
	})

	return bundleVals, nil
}

// SortEntrypointVars sorts the entrypoint variables field.
func (m *EsbuildBundleVarMeta) SortEntrypointVars() {
	slices.SortFunc(m.EntrypointVars, func(a, b *EsbuildEntrypointVar) int {
		return strings.Compare(a.ToEsbuildEntrypointId(""), b.ToEsbuildEntrypointId(""))
	})
}

// ToEsbuildEntrypointId converts an EsbuildEntrypointVar to an esbuild bundle entrypoint id.
func (m *EsbuildEntrypointVar) ToEsbuildEntrypointId(bundleID string) string {
	return bundleID + "/-/" + m.GetPkgImportPath() + "." + m.GetPkgVar()
}
