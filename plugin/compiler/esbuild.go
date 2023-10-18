package bldr_plugin_compiler

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	vardef "github.com/aperturerobotics/bldr/plugin/compiler/vardef"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	esbuild_cli "github.com/evanw/esbuild/pkg/cli"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

// BuildEsbuildBundleMeta builds the bundle metadata from the list of go variable defs.
func BuildEsbuildBundleMeta(
	le *logrus.Entry,
	codeRootPath string,
	codeFiles map[string][]*ast.File,
	fset *token.FileSet,
	pkgs map[string](map[string]*EsbuildDirective),
) (map[string]*EsbuildBundleMeta, error) {
	// bundles is the map of bundle-id to bundle-def
	bundles := make(map[string]*EsbuildBundleMeta)
	getBundle := func(bundleID string) *EsbuildBundleMeta {
		bundleDef := bundles[bundleID]
		if bundleDef != nil {
			return bundleDef
		}

		bundleDef = &EsbuildBundleMeta{Id: bundleID}
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
	for _, bundle := range bundles {
		bundle.SortEntrypointVars()
	}

	return bundles, nil
}

// SortEntrypointVars sorts the entrypoint variables field.
func (m *EsbuildBundleMeta) SortEntrypointVars() {
	slices.SortFunc(m.EntrypointVars, func(a, b *EsbuildEntrypointVar) int {
		sa := a.GetPkgImportPath() + "." + a.GetPkgVar()
		sb := b.GetPkgImportPath() + "." + b.GetPkgVar()
		return strings.Compare(sa, sb)
	})
}

// BuildEsbuildBundle builds an esbuild bundle with the given bundle args.
func BuildEsbuildBundle(
	le *logrus.Entry,
	codeRootPath string,
	meta *EsbuildBundleMeta,
	baseEsbuildOpts *esbuild_api.BuildOptions,
	webPkgs []string,
	outAssetsPath string,
	pluginID string,
	isRelease bool,
) ([]*vardef.PluginVar, []*web_pkg_esbuild.WebPkgRef, []*EsbuildOutputMeta, []string, error) {
	// outputs
	var goVariableDefs []*vardef.PluginVar
	var sourceFilesList []string
	var webPkgRefs []*web_pkg_esbuild.WebPkgRef
	addWebPkgRef := func(webPkgID, webPkgRoot, webPkgSubPath string) {
		webPkgRefs, _ = web_pkg_esbuild.
			WebPkgRefSlice(webPkgRefs).
			AppendWebPkgRef(webPkgID, webPkgRoot, webPkgSubPath)
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

	// merge options set by baseEsbuildOpts
	if baseEsbuildOpts != nil {
		web_pkg_esbuild.MergeEsbuildBuildOpts(buildOpts, baseEsbuildOpts)
	}

	type esbuildBundleVar struct {
		meta           *EsbuildEntrypointVar
		entrypointIdxs []int
	}

	// merge options set by the flags on the comments
	bundleVars := make([]*esbuildBundleVar, 0, len(meta.GetEntrypointVars()))
	for _, varDef := range meta.GetEntrypointVars() {
		esbuildFlags := varDef.GetEsbuildFlags()
		if len(esbuildFlags) == 0 {
			continue
		}

		varBuildOpts, err := esbuild_cli.ParseBuildOptions(varDef.GetEsbuildFlags())
		if err != nil {
			return nil, nil, nil, nil, err
		}

		// note: ignores entrypoints list
		web_pkg_esbuild.MergeEsbuildBuildOpts(buildOpts, &varBuildOpts)

		// build entrypoints list
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
					return nil, nil, nil, nil, err
				}
			} else {
				// determine path relative to the code
				inputPath = filepath.ToSlash(filepath.Join(varDef.PkgCodePath, filepath.Clean(inputPath)))
			}

			// double-check to make sure path is within the code root
			inputPath = filepath.Join(codeRootPath, inputPath)
			inputPath, err = filepath.Rel(codeRootPath, inputPath)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if strings.HasPrefix(inputPath, "../") {
				return nil, nil, nil, nil, errors.Errorf("entrypoint cannot be outside code root: %s", inputPath)
			}

			entryPoints[i].InputPath = inputPath
		}

		// store entrypoints and the indexes within the list
		entrypointIdxs := make([]int, len(entryPoints))
		baseIdx := len(buildOpts.EntryPointsAdvanced)
		for i := 0; i < len(entryPoints); i++ {
			entrypointIdxs[i] = baseIdx + i
		}
		buildOpts.EntryPointsAdvanced = append(buildOpts.EntryPointsAdvanced, entryPoints...)

		// restrict to a single entrypoint per variable.
		if len(entrypointIdxs) != 1 {
			return nil, nil, nil, nil, errors.Errorf(
				"expected single entrypoint but got %v: %s.%s",
				len(entrypointIdxs),
				varDef.PkgImportPath,
				varDef.PkgVar,
			)
		}

		// append the bundle variable definition
		bundleVars = append(bundleVars, &esbuildBundleVar{
			meta:           varDef,
			entrypointIdxs: entrypointIdxs,
		})
	}

	// add the bldr plugin
	buildOpts.Plugins = append(
		buildOpts.Plugins,
		web_pkg_esbuild.BuildEsbuildPlugin(
			le,
			webPkgs,
			addWebPkgRef,
		),
	)

	// https://github.com/evanw/esbuild/issues/1921
	// NOTE: we can't use async import() here since require() is called w/o await.
	externalWebPkgs := slices.Clone(web_pkg_esbuild.BldrExternal)
	externalWebPkgs = append(externalWebPkgs, webPkgs...)
	web_pkg_esbuild.FixEsbuildIssue1921(buildOpts, externalWebPkgs)

	// compile the bundle
	bundleID := meta.GetId()
	le.Debugf("compiling bundle with esbuild: %s", bundleID)
	result := esbuild_api.Build(*buildOpts)
	if err := bldr_esbuild.BuildResultToErr(result); err != nil {
		return nil, nil, nil, nil, err
	}
	if len(result.OutputFiles) == 0 {
		return nil, nil, nil, nil, errors.New("esbuild: expected at least one output file but got none")
	}

	// metaAnalysis contains a graphical view of input files & their sizes
	metaAnalysis := esbuild_api.AnalyzeMetafile(result.Metafile, esbuild_api.AnalyzeMetafileOptions{
		Color: true,
	})
	os.Stderr.WriteString(metaAnalysis + "\n")

	metaFile := &bldr_esbuild.EsbuildMetafile{}
	if err := json.Unmarshal([]byte(result.Metafile), metaFile); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "parse esbuild metafile")
	}

	// Use it to get the list of source files to watch.
	// Note: the paths are relative to the codeRootPath.
	for inFilePath := range metaFile.Inputs {
		sourceFilesList = append(sourceFilesList, inFilePath)
	}

	// write information about outputs to the result
	esbuildOutputMeta := BuildEsbuildOutputMetas(metaFile)

	// transform the paths in the metas to be relative to the assets dir
	for _, meta := range esbuildOutputMeta {
		metaPath := filepath.Join(codeRootPath, meta.Path)
		metaPath, err := filepath.Rel(outAssetsPath, metaPath)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		meta.Path = metaPath
	}

	// re-sort the list
	esbuildOutputMeta = SortEsbuildOutputMetas(esbuildOutputMeta)

	// Match each variable def to an entrypoint.
	for _, bundleVar := range bundleVars {
		// entrypointIdx := varDef.entrypointIdxs[0]
		entrypointDef := buildOpts.EntryPointsAdvanced[bundleVar.entrypointIdxs[0]]
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
			return nil, nil, nil, nil, errors.Errorf("output for entrypoint not found in metafile: %s", entrypointInpPath)
		}

		var outpEntrypointPath string
		var err error
		if entrypointOutp.EntryPoint != "" {
			outpEntrypointPath = filepath.Join(codeRootPath, entrypointOutpPath)
			outpEntrypointPath, err = filepath.Rel(outAssetsPath, outpEntrypointPath)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			outpEntrypointPath = filepath.ToSlash(outpEntrypointPath)
		}
		var outpCssPath string
		if entrypointOutp.CssBundle != "" {
			// NOTE: outp.CssBundle is relative to buildSrcPath
			outpCssPath = filepath.Join(codeRootPath, entrypointOutp.CssBundle)
			outpCssPath, err = filepath.Rel(outAssetsPath, outpCssPath)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			outpCssPath = filepath.ToSlash(outpCssPath)
		}

		// varValue is the value for the go variable.
		varType := bundleVar.meta.GetPkgVarType()
		pkgImportPath := bundleVar.meta.GetPkgImportPath()
		pkgVar := bundleVar.meta.GetPkgVar()
		var varDef *vardef.PluginVar
		switch varType {
		case bldr_esbuild.EsbuildVarType_EsbuildVarType_ENTRYPOINT_PATH:
			var assetHref string
			if outpEntrypointPath != "" {
				assetHref = BuildAssetHref(pluginID, outpEntrypointPath)
			} else {
				assetHref = BuildAssetHref(pluginID, outpCssPath)
			}
			varDef = vardef.NewPluginVar(pkgImportPath, pkgVar, &vardef.PluginVar_StringValue{StringValue: assetHref})
		case bldr_esbuild.EsbuildVarType_EsbuildVarType_ESBUILD_OUTPUT:
			output := &bldr_esbuild.EsbuildOutput{}
			if outpEntrypointPath != "" {
				output.EntrypointHref = BuildAssetHref(pluginID, outpEntrypointPath)
			}
			if outpCssPath != "" {
				output.CssHref = BuildAssetHref(pluginID, outpCssPath)
			}
			varDef = vardef.NewPluginVar(pkgImportPath, pkgVar, &vardef.PluginVar_EsbuildOutput{
				EsbuildOutput: output,
			})
		default:
			return nil, nil, nil, nil, errors.Errorf("unknown target variable type: %s", varType.String())
		}

		goVariableDefs = append(goVariableDefs, varDef)
	}

	return goVariableDefs, webPkgRefs, esbuildOutputMeta, sourceFilesList, nil
}

// BuildEsbuildOutputMetas builds output metadata from the meta file.
func BuildEsbuildOutputMetas(metaFile *bldr_esbuild.EsbuildMetafile) []*EsbuildOutputMeta {
	metas := make([]*EsbuildOutputMeta, 0, len(metaFile.Outputs))
	files := make([]string, 0, 2)
	for outputPath, outputFile := range metaFile.Outputs {
		files = files[:1]
		files[0] = outputPath
		if cssBundlePath := outputFile.CssBundle; cssBundlePath != "" {
			files = append(files, cssBundlePath)
		}
		for _, file := range files {
			metas = append(metas, &EsbuildOutputMeta{
				Path:           file,
				Length:         uint32(outputFile.Bytes),
				EntrypointPath: outputFile.EntryPoint,
			})
		}
	}
	return SortEsbuildOutputMetas(metas)
}

// SortEsbuildOutputMetas sorts and compacts a list of esbuild output meta.
func SortEsbuildOutputMetas(metas []*EsbuildOutputMeta) []*EsbuildOutputMeta {
	slices.SortFunc(metas, func(a, b *EsbuildOutputMeta) int {
		return strings.Compare(a.GetPath(), b.GetPath())
	})
	return slices.CompactFunc(metas, func(a, b *EsbuildOutputMeta) bool {
		return a.GetPath() == b.GetPath()
	})
}
