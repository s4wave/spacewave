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
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

// BuildDefEsbuild builds the list of go variable defs for the given code files.
//
// uses esbuild to compile
func BuildDefEsbuild(
	le *logrus.Entry,
	codeRootPath string,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*EsbuildArgs),
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*GoVarDef, []string, error) {
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

		buildOpts := buildEsbuildBuildOpts(
			codeRootPath,
			outAssetsPath,
			BuildAssetHref(pluginID, ""),
			isRelease,
		)
		bundleDef = &esbuildBundleDef{buildOpts: buildOpts}
		bundles[bundleID] = bundleDef
		return bundleDef
	}

	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, nil, errors.Errorf("failed to find ast.File for package: %s", pkgImportPath)
		}
		pkgCodePath := filepath.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
		relPkgCodePath, err := filepath.Rel(codeRootPath, pkgCodePath)
		if err != nil {
			return nil, nil, errors.Wrap(err, "unable to determine relative path")
		}
		for pkgVar, pkgEsbuildArgs := range pkgVars {
			buildOpts := pkgEsbuildArgs.BuildOpts
			if len(buildOpts.EntryPointsAdvanced) != 0 || len(buildOpts.EntryPoints) != 1 {
				return nil, nil, errors.Errorf("%s: expected single entrypoint", pkgImportPath+"."+pkgVar)
			}

			bundleID := pkgEsbuildArgs.BundleID
			bundleDef := getBundleDef(bundleID)
			// note: ignores the Entrypoint fields
			mergeEsbuildBuildOpts(bundleDef.buildOpts, buildOpts)

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

	// build all bundles
	bundleIDs := maps.Keys(bundles)
	sort.Strings(bundleIDs)
	for _, bundleID := range bundleIDs {
		bundleDef := bundles[bundleID]
		buildOpts := bundleDef.buildOpts

		le.Debugf("compiling bundle with esbuild: %s", bundleID)
		result := esbuild_api.Build(*buildOpts)
		if err := bldr_esbuild.BuildResultToErr(result); err != nil {
			return nil, nil, err
		}
		if len(result.OutputFiles) == 0 {
			return nil, nil, errors.New("esbuild: expected at least one output file but got none")
		}

		// metaAnalysis contains a graphical view of input files & their sizes
		metaAnalysis := esbuild_api.AnalyzeMetafile(result.Metafile, esbuild_api.AnalyzeMetafileOptions{
			Color: true,
		})
		os.Stderr.WriteString(metaAnalysis + "\n")

		metaFile := &EsbuildMetafile{}
		if err := json.Unmarshal([]byte(result.Metafile), metaFile); err != nil {
			return nil, nil, errors.Wrap(err, "parse esbuild metafile")
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
				return nil, nil, errors.Errorf("expected 1 entrypoint idx but got %v", len(varDef.entrypointIdxs))
			}

			// entrypointIdx := varDef.entrypointIdxs[0]
			entrypointDef := varDef.entrypoints[0]
			entrypointInpPath := entrypointDef.InputPath

			// Outputs: the key is the output path relative to the source dir.
			var entrypointOutpPath string
			var entrypointOutp EsbuildMetaFileOutput
			for outpPath, outp := range metaFile.Outputs {
				if outp.EntryPoint == entrypointInpPath {
					entrypointOutpPath = outpPath
					entrypointOutp = outp
					break
				}
			}
			if entrypointOutpPath == "" {
				return nil, nil, errors.Errorf("output for entrypoint not found in metafile: %s", entrypointInpPath)
			}

			var outpEntrypointPath string
			var err error
			if entrypointOutp.EntryPoint != "" {
				outpEntrypointPath = filepath.Join(codeRootPath, entrypointOutpPath)
				outpEntrypointPath, err = filepath.Rel(outAssetsPath, outpEntrypointPath)
				if err != nil {
					return nil, nil, err
				}
				outpEntrypointPath = filepath.ToSlash(outpEntrypointPath)
			}
			var outpCssPath string
			if entrypointOutp.CssBundle != "" {
				// NOTE: outp.CssBundle is relative to buildSrcPath
				outpCssPath = filepath.Join(codeRootPath, entrypointOutp.CssBundle)
				outpCssPath, err = filepath.Rel(outAssetsPath, outpCssPath)
				if err != nil {
					return nil, nil, err
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
				return nil, nil, errors.Errorf("unknown target variable type: %s", varType.String())
			}

			goVariableDefs = append(goVariableDefs, NewGoVarDef(
				varDef.pkgImportPath,
				varDef.pkgVar,
				varValue,
			))
		}
	}
	return goVariableDefs, sourceFilesList, nil
}
