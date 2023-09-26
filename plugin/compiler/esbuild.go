package bldr_plugin_compiler

import (
	"encoding/json"
	"go/ast"
	gast "go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// BuildDefEsbuild builds the list of go variable defs for the given code files.
//
// uses esbuild to compile
func BuildDefEsbuild(
	le *logrus.Entry,
	codeRootPath string,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	baseEsbuildOpts *esbuild_api.BuildOptions,
	pkgs map[string](map[string]*EsbuildArgs),
	webPkgs []string,
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*GoVarDef, []*web_pkg_esbuild.WebPkgRef, []string, error) {
	type esbuildBundleVar struct {
		pkgImportPath string
		pkgVar        string
		esbuildArgs   *EsbuildArgs

		entrypoints    []esbuild_api.EntryPoint
		entrypointIdxs []int
	}

	type esbuildBundleDef struct {
		vars      []*esbuildBundleVar
		buildOpts *esbuild_api.BuildOptions
	}

	bundles := make(map[string]*esbuildBundleDef)
	getBundleDef := func(bundleID string) *esbuildBundleDef {
		bundleDef := bundles[bundleID]
		if bundleDef != nil {
			return bundleDef
		}

		// construct build options
		buildOpts := web_pkg_esbuild.BuildEsbuildBuildOpts(
			le,
			codeRootPath,
			outAssetsPath,
			BuildAssetHref(pluginID, ""),
			isRelease,
			true,
		)

		// https://github.com/evanw/esbuild/issues/1921
		// NOTE: we can't use async import() here since require() is called w/o await.
		FixEsbuildIssue1921(buildOpts)

		// merge options set by baseEsbuildOpts
		if baseEsbuildOpts != nil {
			web_pkg_esbuild.MergeEsbuildBuildOpts(buildOpts, baseEsbuildOpts)
		}

		bundleDef = &esbuildBundleDef{buildOpts: buildOpts}
		bundles[bundleID] = bundleDef
		return bundleDef
	}

	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, nil, nil, errors.Errorf("failed to find ast.File for package: %s", pkgImportPath)
		}
		pkgCodePath := filepath.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
		relPkgCodePath, err := filepath.Rel(codeRootPath, pkgCodePath)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "unable to determine relative path")
		}
		for pkgVar, pkgEsbuildArgs := range pkgVars {
			buildOpts := pkgEsbuildArgs.BuildOpts
			if len(buildOpts.EntryPointsAdvanced) != 0 || len(buildOpts.EntryPoints) != 1 {
				return nil, nil, nil, errors.Errorf("%s: expected single entrypoint", pkgImportPath+"."+pkgVar)
			}

			bundleID := pkgEsbuildArgs.BundleID
			bundleDef := getBundleDef(bundleID)

			// note: ignores the Entrypoint fields
			web_pkg_esbuild.MergeEsbuildBuildOpts(bundleDef.buildOpts, buildOpts)

			adjustPath := func(entryPointPath string) string {
				// ignore absolute paths (strip / prefix)
				for strings.HasPrefix(entryPointPath, "/") {
					entryPointPath = entryPointPath[1:]
				}
				// determine path relative to the project root
				adjPath := filepath.Join(relPkgCodePath, filepath.Clean(entryPointPath))
				return filepath.ToSlash(adjPath)
			}

			// note: we only allow 1 entrypoint currently
			entryPoints := make([]esbuild_api.EntryPoint, len(buildOpts.EntryPointsAdvanced))
			copy(entryPoints, buildOpts.EntryPointsAdvanced)

			// convert entrypoints to advanced entrypoints
			for _, entrypointPath := range buildOpts.EntryPoints {
				entryPoints = append(entryPoints, esbuild_api.EntryPoint{
					InputPath: entrypointPath,
				})
			}

			// transform entrypoint paths to be relative to codeRootPath
			for i := range entryPoints {
				entryPoints[i].InputPath = adjustPath(entryPoints[i].InputPath)
			}

			// store entrypoints
			entrypointIdxs := make([]int, len(entryPoints))
			baseIdx := len(bundleDef.buildOpts.EntryPointsAdvanced)
			for i := 0; i < len(entryPoints); i++ {
				entrypointIdxs[i] = baseIdx + i
			}
			bundleDef.buildOpts.EntryPointsAdvanced = append(bundleDef.buildOpts.EntryPointsAdvanced, entryPoints...)

			// store variable definitions
			bundleDef.vars = append(bundleDef.vars, &esbuildBundleVar{
				pkgImportPath:  pkgImportPath,
				pkgVar:         pkgVar,
				esbuildArgs:    pkgEsbuildArgs,
				entrypoints:    bundleDef.buildOpts.EntryPointsAdvanced[entrypointIdxs[0] : entrypointIdxs[len(entrypointIdxs)-1]+1],
				entrypointIdxs: entrypointIdxs,
			})
		}
	}

	// outputs
	var goVariableDefs []*GoVarDef
	var sourceFilesList []string
	var webPkgRefs []*web_pkg_esbuild.WebPkgRef
	addWebPkgRef := func(webPkgID, webPkgRoot, webPkgSubPath string) {
		webPkgRefs = web_pkg_esbuild.AddWebPkgRef(webPkgRefs, webPkgID, webPkgRoot, webPkgSubPath)
	}

	// build list of packages to externalize
	extWebPkgs := slices.Clone(webPkgs)
	slices.Sort(extWebPkgs)
	extWebPkgs = slices.Compact(extWebPkgs)

	// build all bundles
	bundleIDs := maps.Keys(bundles)
	sort.Strings(bundleIDs)
	for _, bundleID := range bundleIDs {
		bundleDef := bundles[bundleID]
		buildOpts := *bundleDef.buildOpts
		buildOpts.Plugins = slices.Clone(buildOpts.Plugins)

		// add the bldr plugin
		buildOpts.Plugins = append(
			buildOpts.Plugins,
			web_pkg_esbuild.BuildEsbuildPlugin(
				le,
				extWebPkgs,
				addWebPkgRef,
			),
		)

		le.Debugf("compiling bundle with esbuild: %s", bundleID)
		result := esbuild_api.Build(buildOpts)
		if err := bldr_esbuild.BuildResultToErr(result); err != nil {
			return nil, nil, nil, err
		}
		if len(result.OutputFiles) == 0 {
			return nil, nil, nil, errors.New("esbuild: expected at least one output file but got none")
		}

		// metaAnalysis contains a graphical view of input files & their sizes
		metaAnalysis := esbuild_api.AnalyzeMetafile(result.Metafile, esbuild_api.AnalyzeMetafileOptions{
			Color: true,
		})
		os.Stderr.WriteString(metaAnalysis + "\n")

		metaFile := &bldr_esbuild.EsbuildMetafile{}
		if err := json.Unmarshal([]byte(result.Metafile), metaFile); err != nil {
			return nil, nil, nil, errors.Wrap(err, "parse esbuild metafile")
		}

		// Use it to get the list of source files to watch.
		// Note: the paths are relative to the codeRootPath.
		for inFilePath := range metaFile.Inputs {
			sourceFilesList = append(sourceFilesList, inFilePath)
		}

		// Match each variable def to an entrypoint.
		for _, varDef := range bundleDef.vars {
			// NOTE; we restrict to a single entrypoint for now.
			if len(varDef.entrypointIdxs) != 1 {
				return nil, nil, nil, errors.Errorf("expected 1 entrypoint idx but got %v", len(varDef.entrypointIdxs))
			}

			// entrypointIdx := varDef.entrypointIdxs[0]
			entrypointDef := varDef.entrypoints[0]
			entrypointInpPath := entrypointDef.InputPath

			// Outputs: the key is the output path relative to the source dir.
			var entrypointOutpPath string
			var entrypointOutp bldr_esbuild.EsbuildMetaFileOutput
			for outpPath, outp := range metaFile.Outputs {
				if outp.EntryPoint == entrypointInpPath {
					entrypointOutpPath = outpPath
					entrypointOutp = outp
					break
				}
			}
			if entrypointOutpPath == "" {
				return nil, nil, nil, errors.Errorf("output for entrypoint not found in metafile: %s", entrypointInpPath)
			}

			var outpEntrypointPath string
			var err error
			if entrypointOutp.EntryPoint != "" {
				outpEntrypointPath = filepath.Join(codeRootPath, entrypointOutpPath)
				outpEntrypointPath, err = filepath.Rel(outAssetsPath, outpEntrypointPath)
				if err != nil {
					return nil, nil, nil, err
				}
				outpEntrypointPath = filepath.ToSlash(outpEntrypointPath)
			}
			var outpCssPath string
			if entrypointOutp.CssBundle != "" {
				// NOTE: outp.CssBundle is relative to buildSrcPath
				outpCssPath = filepath.Join(codeRootPath, entrypointOutp.CssBundle)
				outpCssPath, err = filepath.Rel(outAssetsPath, outpCssPath)
				if err != nil {
					return nil, nil, nil, err
				}
				outpCssPath = filepath.ToSlash(outpCssPath)
			}

			buildStringLit := func(lit string) *gast.BasicLit {
				return &gast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(lit),
				}
			}

			// varValue is the value for the go variable.
			varType := varDef.esbuildArgs.EsbuildVarType
			var varValue gast.Expr
			switch varType {
			case bldr_esbuild.EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH:
				if outpEntrypointPath != "" {
					varValue = buildStringLit(BuildAssetHref(pluginID, outpEntrypointPath))
				} else {
					varValue = buildStringLit(BuildAssetHref(pluginID, outpCssPath))
				}
			case bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT:
				elts := make([]gast.Expr, 0, 2)
				if outpEntrypointPath != "" {
					elts = append(elts, &gast.KeyValueExpr{
						Key:   gast.NewIdent("EntrypointHref"),
						Value: buildStringLit(BuildAssetHref(pluginID, outpEntrypointPath)),
					})
				}
				if outpCssPath != "" {
					elts = append(elts, &gast.KeyValueExpr{
						Key:   gast.NewIdent("CssHref"),
						Value: buildStringLit(BuildAssetHref(pluginID, outpCssPath)),
					})
				}
				varValue = &gast.CompositeLit{
					Elts: elts,
					Type: &gast.SelectorExpr{
						Sel: gast.NewIdent("EsbuildOutput"),
						X:   gast.NewIdent("bldr_values"),
					},
				}
			default:
				return nil, nil, nil, errors.Errorf("unknown target variable type: %s", varType.String())
			}

			goVariableDefs = append(goVariableDefs, NewGoVarDef(
				varDef.pkgImportPath,
				varDef.pkgVar,
				varValue,
			))
		}
	}

	return goVariableDefs, webPkgRefs, sourceFilesList, nil
}

// FixEsbuildIssue1921 fixes externalized esbuild imports failing with compiled commonjs modules.
//
// https://github.com/evanw/esbuild/issues/1921
func FixEsbuildIssue1921(opts *esbuild_api.BuildOptions) {
	if opts.Banner == nil {
		opts.Banner = make(map[string]string, 1)
	}
	old := opts.Banner["js"]
	if len(old) != 0 {
		old += "\n"
	}
	opts.Banner["js"] = old + bldrBuiltInsRequireShim
}

const bldrBuiltInsRequireShim = `
import * as __bldr_React from 'react';
import * as __bldr_ReactJsxRuntime from 'react/jsx-runtime';
import * as __bldr_ReactDomIndex from 'react-dom';
import * as __bldr_ReactDomClient from 'react-dom/client';
import * as __bldr_AptreBldr from '@aptre/bldr';
import * as __bldr_AptreBldrReact from '@aptre/bldr-react';
const require = (pkgName) => {
  switch (pkgName) {
  case 'react':
    return __bldr_React;
  case 'react/jsx-runtime':
    return __bldr_ReactJsxRuntime;
  case 'react-dom':
    return __bldr_ReactDomIndex;
  case 'react-dom/client':
    return __bldr_ReactDomClient;
  case '@aptre/bldr':
    return __bldr_AptreBldr;
  case '@aptre/bldr-react':
    return __bldr_AptreBldrReact;
  default:
    throw Error('Dynamic require of "' + pkgName + '" is not supported: see esbuild issue 1921.');
  }
};
`
