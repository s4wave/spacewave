package plugin_compiler

import (
	"encoding/json"
	"go/ast"
	gast "go/ast"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/esbuild"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	esbuild_cli "github.com/evanw/esbuild/pkg/cli"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EsbuildTag is the comment tag used for esbuild.
const EsbuildTag = "bldr:esbuild"

// DefaultBundleID is the default ID to use for esbuild bundles.
const DefaultBundleID = "default"

// BundleIDFlag is the flag for bundle-id.
const BundleIDFlag = "--bundle-id="

// EsbuildArgs are arguments parsed from a bldr:esbuild directive.
type EsbuildArgs struct {
	// BundleID is the bundle identifier to use for esbuild.
	// If unset, uses "default".
	BundleID string
	// BuildOpts are the esbuild build options.
	// Note that all BuildOptions for the same BundleID are merged.
	BuildOpts *esbuild_api.BuildOptions
	// EsbuildVarType is the type of esbuild output variable we are using.
	EsbuildVarType bldr_esbuild.EsbuildVarType
}

// TrimEsbuildArgs trims the bldr:esbuild prefix from a string.
// Returns if the string had the prefix.
func TrimEsbuildArgs(value string) (string, bool) {
	return TrimCommentArgs(EsbuildTag, value)
}

// ParseEsbuildComments parses the bldr:esbuild directive comments.
//
// If no bldr:esbuild prefix is found, returns nil, false, nil
func ParseEsbuildComments(values []string, spec *ast.ValueSpec) (*EsbuildArgs, bool, error) {
	args, found, err := CombineShellComments(EsbuildTag, values)
	if err != nil || !found {
		return nil, found, err
	}

	// determine bundle id from the args
	bundleID := DefaultBundleID
	for _, arg := range args {
		if strings.HasPrefix(arg, BundleIDFlag) {
			value := arg[len(BundleIDFlag):]
			if len(value) != 0 {
				bundleID = value
			}
		}
	}

	// parse esbuild cli args
	buildOpts, err := esbuild_cli.ParseBuildOptions(args)
	if err != nil {
		return nil, true, err
	}

	// determine the variable type for the Esbuild variable
	var varType bldr_esbuild.EsbuildVarType
	typeStr := types.ExprString(spec.Type)
	switch typeStr {
	case "string":
		varType = bldr_esbuild.EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH
	case "bldr_esbuild.EsbuildOutput":
		varType = bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT
	default:
		return nil, true, errors.Errorf("unexpected type for bldr:esbuild variable: %s", typeStr)
	}

	return &EsbuildArgs{
		BundleID:       bundleID,
		BuildOpts:      &buildOpts,
		EsbuildVarType: varType,
	}, true, nil
}

// FindEsbuildVariables searches for bldr:esbuild comments.
func (a *Analysis) FindEsbuildVariables(codeFiles map[string][]*ast.File) (map[string](map[string]*EsbuildArgs), error) {
	return FindTagComments(EsbuildTag, a.fset, codeFiles, ParseEsbuildComments)
}

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
	var esbuildArgs []*EsbuildArgs
	var esbuildBuildVars []string
	var esbuildBuildPkgs []string
	var esbuildBuildPaths []string

	type esbuildBundleVar struct {
		pkgImportPath string
		pkgVar        string
		esbuildArgs   *EsbuildArgs
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
		for pkgVar, pkgEsbuildArgs := range pkgVars {
			buildOpts := pkgEsbuildArgs.BuildOpts
			if len(buildOpts.EntryPointsAdvanced) != 0 || len(buildOpts.EntryPoints) != 1 {
				return nil, nil, errors.Errorf("%s: expected single entrypoint", pkgImportPath+"."+pkgVar)
			}

			bundleID := pkgEsbuildArgs.BundleID
			bundleDef := getBundleDef(bundleID)
			mergeEsbuildBuildOpts(bundleDef.buildOpts, buildOpts)

			bundleDef.vars = append(bundleDef.vars, &esbuildBundleVar{
				pkgImportPath: pkgImportPath,
				pkgVar:        pkgVar,
				esbuildArgs:   pkgEsbuildArgs,
			})

			// TODO add entrypoint
			// bundleDef.buildOpts.EntryPointsAdvanced =
		}
	}

	var goVariableDefs []*GoVarDef
	var sourceFilesList []string
	// bundles := make(map[string])
	for pkgImportPath, pkgVars := range pkgs {
		pkgCodeFiles := codeFiles[pkgImportPath]
		if len(pkgCodeFiles) == 0 {
			return nil, nil, errors.Errorf("failed to find corresponding ast.File for package: %s", pkgImportPath)
		}
		for pkgVar, pkgEsbuildArgs := range pkgVars {
			buildOpts := pkgEsbuildArgs.BuildOpts
			if len(buildOpts.EntryPointsAdvanced) != 0 || len(buildOpts.EntryPoints) != 1 {
				return nil, nil, errors.Errorf("%s: expected single entrypoint", pkgImportPath+"."+pkgVar)
			}

			// platform / target
			buildOpts.Platform = esbuild_api.PlatformBrowser
			buildOpts.Format = esbuild_api.FormatESModule
			if buildOpts.Target == 0 {
				buildOpts.Target = esbuild_api.ES2021
			}

			if !isRelease && buildOpts.Sourcemap == 0 {
				buildOpts.Sourcemap = esbuild_api.SourceMapInline
			}

			// TODO: add plugin to use common import for some packages like React.

			// other common settings
			pkgCodePath := path.Dir(fset.File(pkgCodeFiles[0].Pos()).Name())
			buildOpts.AbsWorkingDir = pkgCodePath
			buildOpts.LogLevel = esbuild_api.LogLevelDebug
			buildOpts.Outfile, buildOpts.Outbase = "", ""
			buildOpts.Outdir = outAssetsPath
			buildOpts.PublicPath = BuildAssetHref(pluginID, "")
			buildOpts.TreeShaking = esbuild_api.TreeShakingTrue

			buildOpts.AllowOverwrite = true
			buildOpts.Bundle = true
			buildOpts.Metafile = true
			buildOpts.Write = true
			buildOpts.Splitting = true

			// ensure that we reload scripts when they change
			buildOpts.EntryNames = "[dir]/[name]-[hash]"

			// add common loader types
			if buildOpts.Loader == nil {
				buildOpts.Loader = make(map[string]esbuild_api.Loader)
			}
			addLoader := func(ext string, typ esbuild_api.Loader) {
				if _, ok := buildOpts.Loader[ext]; !ok {
					buildOpts.Loader[ext] = typ
				}
			}
			useFileLoader := []string{"woff", "woff2", "png", "jpg", "jpeg", "svg", "gif", "tif", "tiff"}
			for _, ext := range useFileLoader {
				addLoader("."+ext, esbuild_api.LoaderFile)
			}

			esbuildArgs = append(esbuildArgs, pkgEsbuildArgs)
			esbuildBuildVars = append(esbuildBuildVars, pkgVar)
			esbuildBuildPkgs = append(esbuildBuildPkgs, pkgImportPath)
			esbuildBuildPaths = append(esbuildBuildPaths, pkgCodePath)
		}
	}
	for i, buildArgs := range esbuildArgs {
		buildSrcPath := esbuildBuildPaths[i]
		buildOpts := buildArgs.BuildOpts

		// we currently enforce 1 entrypoint above
		entrypointFilename := buildOpts.EntryPoints[0]
		le.Debugf("compiling file with esbuild: %s", entrypointFilename)

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
		// Note: the paths are relative to the package code path.
		for inFileRelPath := range metaFile.Inputs {
			inFilePath := path.Join(buildSrcPath, inFileRelPath)
			sourceFilesList = append(sourceFilesList, inFilePath)
		}

		// Outputs: the key is the output path relative to the source dir.
		var entrypointOutpPath string
		var entrypointOutp EsbuildMetaFileOutput
		for outpPath, outp := range metaFile.Outputs {
			if outp.EntryPoint != "" {
				entrypointOutpPath = outpPath
				entrypointOutp = outp
				break
			}
		}
		if entrypointOutpPath == "" {
			return nil, nil, errors.New("output for entrypoint not found in metafile")
		}

		var outpEntrypointPath string
		var err error
		if entrypointOutp.EntryPoint != "" {
			outpEntrypointPath = path.Join(buildSrcPath, entrypointOutpPath)
			outpEntrypointPath, err = filepath.Rel(outAssetsPath, outpEntrypointPath)
			if err != nil {
				return nil, nil, err
			}
		}
		var outpCssPath string
		if entrypointOutp.CssBundle != "" {
			// NOTE: outp.CssBundle is relative to buildSrcPath
			outpCssPath = path.Join(buildSrcPath, entrypointOutp.CssBundle)
			outpCssPath, err = filepath.Rel(outAssetsPath, outpCssPath)
			if err != nil {
				return nil, nil, err
			}
		}

		buildStringLit := func(lit string) *gast.BasicLit {
			return &gast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(lit),
			}
		}

		// varValue is the value for the go variable.
		var varValue gast.Expr
		switch buildArgs.EsbuildVarType {
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
			return nil, nil, errors.Errorf("unknown target variable type: %s", buildArgs.EsbuildVarType.String())
		}

		goVariableDefs = append(goVariableDefs, NewGoVarDef(
			esbuildBuildPkgs[i],
			esbuildBuildVars[i],
			varValue,
		))
	}
	return goVariableDefs, sourceFilesList, nil
}
