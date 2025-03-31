package web_pkg_esbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
	determine_cjs_exports_exec "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports/exec"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BldrExternal are packages that are bundled externally for all bldr components.
var BldrExternal = []string{"react", "react-dom", "@aptre/bldr", "@aptre/bldr-react", "@aptre/protobuf-es-lite"}

// GetBldrExternalWebPkgRefs returns the web pkg refs for BldrExternal.
func GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot string) []*web_pkg.WebPkgRef {
	return []*web_pkg.WebPkgRef{{
		WebPkgId:   "react",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react"),
		Imports:    []string{"index.js", "jsx-runtime.js"},
	}, {
		WebPkgId:   "react-dom",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react-dom"),
		Imports:    []string{"index.js", "client.js"},
	}, {
		WebPkgId:   "@aptre/bldr",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr-react"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgId:   "@aptre/protobuf-es-lite",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/@aptre/protobuf-es-lite/dist"),
		Imports:    []string{"index.js"},
	}}
}

// pkgRootAlias is an alias to the root of the bldr web pkg.
const pkgRootAlias = "@bldr-web-pkg"

// https://github.com/evanw/esbuild/issues/1921
// NOTE: we can't use async import() here since require() is called w/o await.
/*
const reactDomImportShim = `
import * as __bldr_React from 'react';
const require = (pkgName) => {
  switch (pkgName) {
  case 'react':
    return __bldr_React;
  default:
    throw Error('Dynamic require of "' + pkgName + '" within react-dom is not supported');
  }
};
`
*/

// ResolveWebPkgRefsEsbuild resolves the WebPkgRef list and ensures that all
// necessary imports are listed within the refs list. It also computes the Refs
// field on each WebPkgRef.
//
// This function solves the case where a WebPkg within the bundle references
// another WebPkg within the bundle. Initially the webPkgsRefs list will contain
// only references made from within the plugin we are currently compiling. This
// function will repeatedly resolve each WebPkgRef and search for references to
// other WebPkg. If the WebPkg references add an additional entrypoint to the
// WebPkgRef that was not previously referenced within the WebPkgRef, the
// entrypoint will be added and the WebPkgRef will be queued for scanning again.
//
// Note: the refs slice will be edited in-place.
func ResolveWebPkgRefsEsbuild(
	ctx context.Context,
	le *logrus.Entry,
	codeRootPath string,
	webPkgsRefs []*web_pkg.WebPkgRef,
) ([]*web_pkg.WebPkgRef, error) {
	// stack contains the list of refs we need to process
	// sort by web pkg id
	stack := slices.Clone(webPkgsRefs)
	sortStack := func() {
		web_pkg.SortWebPkgRefs(stack)
	}
	sortStack()

	// process repeatedly
	for len(stack) != 0 {
		// dequeue next ref
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// use esbuild to determine the list of web pkg references
		// webPkgRoot := ref.WebPkgRoot
		webPkgID := ref.WebPkgId
		buildOpts := BuildEsbuildBuildOpts(
			le,
			codeRootPath, // resolve relative to project root for node_modules
			"",           // empty output path
			"",           // empty public path
			false,        // set isRelease to false
			false,        // skip using file hashes
		)

		// clear all output
		buildOpts.Outdir = "./" // note: not actually written since write=false.
		buildOpts.Write = false
		buildOpts.Metafile = false
		buildOpts.Splitting = false

		// disable too verbose logging (for resolving web pkg refs)
		// buildOpts.LogLevel = esbuild_api.LogLevelError
		buildOpts.LogLevel = esbuild_api.LogLevelVerbose

		// disable some unnecessary processing
		buildOpts.Sourcemap = esbuild_api.SourceMapNone
		buildOpts.TreeShaking = esbuild_api.TreeShakingFalse

		// alias + out ext
		buildOpts.OutExtension = map[string]string{".js": ".mjs"}
		buildOpts.Alias[pkgRootAlias] = ref.WebPkgRoot
		buildOpts.Alias[webPkgID] = ref.WebPkgRoot

		// ensure external does not contain webPkgID or any of the other refs we need to resolve
		// we need to resolve the actual paths to the web pkg files
		buildOpts.External = slices.DeleteFunc(buildOpts.External, func(v string) bool {
			if v == webPkgID {
				return true
			}

			return slices.Contains(BldrExternal, v)
		})

		// configure entrypoints
		buildEntrypoints := make([]esbuild_api.EntryPoint, len(ref.Imports))
		for i, impPath := range ref.Imports {
			webPkgImpPath := path.Join(webPkgID, impPath)
			buildEntrypoints[i] = esbuild_api.EntryPoint{
				InputPath:  webPkgImpPath,
				OutputPath: impPath[:len(impPath)-len(path.Ext(impPath))],
			}
		}
		buildOpts.EntryPointsAdvanced = buildEntrypoints
		buildOpts.EntryPoints = nil

		// build full list of web pkgs so far
		webPkgIDs := make([]string, len(webPkgsRefs))
		for i, ref := range webPkgsRefs {
			webPkgIDs[i] = ref.WebPkgId
		}
		slices.Sort(webPkgIDs)
		webPkgIDs = slices.Compact(webPkgIDs)

		// clear the CrossRefs field on the Ref
		// we will re-build this slice below
		ref.CrossRefs = nil

		// when we find a web pkg ref we can add it to the list & queue for processing
		addWebPkgRef := func(webPkgID, webPkgRoot, webPkgSubPath string) {
			if !slices.Contains(ref.CrossRefs, webPkgID) {
				ref.CrossRefs = append(ref.CrossRefs, webPkgID)
			}

			var dirty bool
			webPkgsRefs, dirty = web_pkg.WebPkgRefSlice(webPkgsRefs).AppendWebPkgRef(webPkgID, webPkgRoot, webPkgSubPath)
			if dirty {
				le.WithFields(logrus.Fields{
					"web-pkg-id":         ref.WebPkgId,
					"ref-web-pkg-id":     webPkgID,
					"ref-web-pkg-import": webPkgSubPath,
				}).Debug("added web pkg ref")
				changedRef, _ := web_pkg.FindWebPkgRef(webPkgsRefs, webPkgID)
				if changedRef != nil && !slices.Contains(stack, changedRef) {
					stack = append(stack, changedRef)
					sortStack()
				}
			}
		}

		// add plugin to scan for web pkg imports & update the refs list
		buildOpts.Plugins = append(
			buildOpts.Plugins,
			BuildEsbuildPlugin(
				le,
				webPkgIDs,
				addWebPkgRef,
			),
		)

		le.
			WithField("web-pkg-id", webPkgID).
			WithField("web-pkg-imports", ref.Imports).
			Debug("analyzing web pkg bundle with esbuild")
		result := esbuild_api.Build(*buildOpts)
		if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
			return nil, err
		}
	}

	return webPkgsRefs, nil
}

// BuildWebPkgsEsbuild builds the WebPkg bundle from the list of web package ids.
//
// uses esbuild to compile and the esbuild resolution algorithm
// stores the files in the output path using web_pkg io/fs format
// bundles the entrypoints defined for the target environment (browser or node)
// returns the list of build web pkg ids
// returns the source files list (usually files in node_modules)
// webPkgBasePath is the base URL path for the web packages, e.x. /b/pkg
func BuildWebPkgsEsbuild(
	ctx context.Context,
	le *logrus.Entry,
	codeRootPath string,
	webPkgsRefs []*web_pkg.WebPkgRef,
	outputPath string,
	webPkgBasePath string,
	isRelease bool,
) (webPkgIDs, sourcePaths []string, err error) {
	// Build list of web pkg IDs
	for _, webPkgRef := range webPkgsRefs {
		webPkgIDs = append(webPkgIDs, webPkgRef.WebPkgId)
	}
	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	// NOTE: esbuild removes the named exports when bundling libraries like
	// React which contain commonjs-like constructions for building the
	// module.exports named export set. To fix this issue, we will run a script
	// which uses cjs-module-lexer to parse the module and determine the list of
	// named exports. We then generate an entrypoint js file which re-exports
	// the list of named exports using a pure esm format that Esbuild properly
	// converts to a list of named exports along with bundling the library.
	//
	// https://github.com/evanw/esbuild/issues/442#issuecomment-739340295

	// For each web pkg reference (import):
	//  - Build a list of imports (entrypoints) to pass to esbuild
	//  - Run the cjs lexer to determine the list of named exports
	//  - Run esbuild to bundle the package + named exports to out
	var sourceFilesList []string
	bldrEsbuildTmpDir := "__bldr_esbuild_tmp"
	for _, webPkgRef := range webPkgsRefs {
		webPkgID := webPkgRef.WebPkgId
		pkgOutputPath := filepath.Join(outputPath, webPkgID)
		if _, err := os.Stat(pkgOutputPath); !os.IsNotExist(err) {
			if err := os.RemoveAll(pkgOutputPath); err != nil {
				return nil, nil, err
			}
		}

		pkgPublicPath := path.Join(webPkgBasePath, webPkgID)
		if strings.HasPrefix(webPkgBasePath, "./") {
			pkgPublicPath = "./" + pkgPublicPath
		}

		// Create a temporary dir for the entrypoints
		pkgBuildPath := webPkgRef.WebPkgRoot
		pkgTmpPath := filepath.Join(pkgOutputPath, bldrEsbuildTmpDir)
		if err := os.MkdirAll(pkgTmpPath, 0o755); err != nil {
			return nil, nil, err
		}

		le.
			WithField("web-pkg-id", webPkgID).
			WithField("web-pkg-imports", webPkgRef.Imports).
			Debug("analyzing web pkg bundle exports")

		// Determine the list of exports for each of the imports.
		buildEntrypoints := make([]esbuild_api.EntryPoint, len(webPkgRef.Imports))
		origEntrypointImpPaths := make(map[string]string)
		for i, impPath := range webPkgRef.Imports {
			// webPkgImpPath := filepath.Join(webPkgRoot, impPath)
			webPkgImpPath := path.Join(webPkgID, impPath)
			impOutPath := filepath.Join(pkgOutputPath, impPath)

			// we should only process js-like files
			// pass other imports directly to esbuild as-is
			ext := filepath.Ext(webPkgImpPath)
			if !determine_cjs_exports.SupportsExtension(ext) {
				// strip the output file extension, esbuild will add it automatically
				buildEntrypoints[i] = esbuild_api.EntryPoint{
					InputPath:  webPkgImpPath,
					OutputPath: impOutPath[:len(impOutPath)-len(path.Ext(impOutPath))],
				}
				continue
			}

			webPkgExports, err := determine_cjs_exports_exec.ExecDetermineCjsExports(
				ctx,
				le.WithFields(logrus.Fields{
					"exec":           "determine-cjs-exports",
					"web-pkg-id":     webPkgID,
					"web-pkg-import": impPath,
				}),
				webPkgRef.WebPkgRoot, // codeRootPath,
				"./"+impPath,
			)
			if err != nil {
				return nil, nil, err
			}

			webPkgEntrypointScript := determine_cjs_exports.GenerateRemapExports(
				path.Join(pkgRootAlias, impPath),
				webPkgExports,
			)

			// write the entrypoint file
			outEntrypointPath := filepath.Join(pkgTmpPath, impPath)
			if outEntrypointExt := filepath.Ext(outEntrypointPath); outEntrypointExt == ".js" {
				outEntrypointPath = outEntrypointPath[:len(outEntrypointPath)-len(outEntrypointExt)] + ".mjs"
			}

			// remap the output file extension to .mjs
			impOutExt := filepath.Ext(impOutPath)

			// strip the output file extension, esbuild will add it automatically
			impOutPath = impOutPath[:len(impOutPath)-len(impOutExt)] // + ".mjs"

			outEntrypointDir := filepath.Dir(outEntrypointPath)
			if err := os.MkdirAll(outEntrypointDir, 0o755); err != nil {
				return nil, nil, err
			}

			err = os.WriteFile(outEntrypointPath, []byte(webPkgEntrypointScript+"\n"), 0o644)
			if err != nil {
				return nil, nil, err
			}

			impEntrypointPath, err := filepath.Rel(pkgBuildPath, outEntrypointPath)
			if err != nil {
				return nil, nil, err
			}

			// add the entrypoint
			origEntrypointImpPaths[impEntrypointPath] = webPkgImpPath
			buildEntrypoints[i] = esbuild_api.EntryPoint{
				InputPath:  impEntrypointPath,
				OutputPath: impOutPath,
			}
		}

		le.
			WithField("web-pkg-id", webPkgID).
			WithField("web-pkg-imports", webPkgRef.Imports).
			Debug("building web pkg bundle with esbuild")

		buildOpts := BuildEsbuildBuildOpts(
			le,
			pkgBuildPath, // codeRootPath,
			pkgOutputPath,
			pkgPublicPath,
			isRelease,
			false,
		)

		buildOpts.EntryPoints = nil
		buildOpts.EntryPointsAdvanced = buildEntrypoints
		// buildOpts.Sourcemap = esbuild_api.SourceMapNone - set by isRelease in BuildEsbuildBuildOpts
		buildOpts.TreeShaking = esbuild_api.TreeShakingFalse
		buildOpts.OutExtension = map[string]string{".js": ".mjs"}
		buildOpts.Alias[pkgRootAlias] = webPkgRef.WebPkgRoot
		buildOpts.Alias[webPkgID] = webPkgRef.WebPkgRoot

		// see: https://github.com/evanw/esbuild/issues/399
		buildOpts.Splitting = false

		// externalize some packages we remap with an import map
		for _, toExternalize := range BldrExternal {
			if webPkgID != toExternalize && !slices.Contains(buildOpts.External, toExternalize) {
				buildOpts.External = append(buildOpts.External, toExternalize)
			}
		}

		// ensure external does not contain any of the web pkgs ids
		// we need to resolve the actual paths to the web pkg files
		/*
			buildOpts.External = slices.DeleteFunc(buildOpts.External, func(v string) bool {
				if v == webPkgID {
					return true
				}

				return slices.Contains(BldrExternal, v)
			})
		*/

		// add plugin to rewrite peer web pkg imports
		// we expect that the web pkgs ids list has been previously resolved by ResolveWebPkgsEsbuild
		// remove the current web pkg from the list
		webPkgIDsExclCurr := slices.DeleteFunc(slices.Clone(webPkgIDs), func(id string) bool {
			return id == webPkgID
		})
		webPkgIDsExclCurrAndExternal := slices.DeleteFunc(slices.Clone(webPkgIDsExclCurr), func(id string) bool {
			return slices.Contains(BldrExternal, id)
		})

		buildOpts.Plugins = append(
			buildOpts.Plugins,
			BuildEsbuildPlugin(
				le,
				webPkgIDsExclCurrAndExternal,
				nil, // addWebPkgRef,
			),
		)

		// add banner
		msg := fmt.Sprintf("built by bldr/web/pkg: %s/%v", webPkgID, webPkgRef.Imports)
		buildOpts.Banner["js"] = "// " + msg
		buildOpts.Banner["css"] = "/* " + msg + " */"

		// add import shim for common-js support
		// HACK: exclude react to avoid issues w/ secret internals
		if webPkgID != "react" {
			FixEsbuildIssue1921(buildOpts)
		}

		le.Debugf("compiling web pkg bundle with esbuild: %s", webPkgID)
		result := esbuild_api.Build(*buildOpts)
		if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
			return nil, nil, err
		}
		if len(result.OutputFiles) == 0 {
			return nil, nil, errors.New("esbuild: expected at least one output file but got none")
		}

		// metaAnalysis contains a graphical view of input files & their sizes
		/*
			metaAnalysis := esbuild_api.AnalyzeMetafile(result.Metafile, esbuild_api.AnalyzeMetafileOptions{
				Color: true,
			})
			os.Stderr.WriteString(metaAnalysis + "\n")
		*/

		metaFile := &bldr_esbuild_build.EsbuildMetafile{}
		if err := json.Unmarshal([]byte(result.Metafile), metaFile); err != nil {
			return nil, nil, errors.Wrap(err, "parse esbuild metafile")
		}

		// Clear the temporary entrypoint sources path.
		if err := os.RemoveAll(pkgTmpPath); err != nil {
			return nil, nil, err
		}

		// Use it to get the list of source files to watch.
		// Note: the paths are relative to the codeRootPath.
		for inFilePath := range metaFile.Inputs {
			if origPath := origEntrypointImpPaths[inFilePath]; origPath != "" {
				inFilePath = origPath
			}

			// Join the file path with the esbuild working directory.
			inFilePath = filepath.Join(pkgBuildPath, inFilePath)

			// Transform it to be relative to the code root.
			inFilePath, err = filepath.Rel(codeRootPath, inFilePath)
			if err != nil {
				return nil, nil, err
			}

			// If the file doesn't exist, skip it.
			inFileAbsPath := filepath.Join(codeRootPath, inFilePath)
			if _, err := os.Stat(inFileAbsPath); err != nil {
				if !os.IsNotExist(err) {
					return nil, nil, err
				}
				continue
			}

			// If the file has the tmp dir within it, skip it.
			if strings.Contains(inFilePath, bldrEsbuildTmpDir) {
				continue
			}

			// Append it to the source files list.
			sourceFilesList = append(sourceFilesList, inFilePath)
		}
	}

	slices.Sort(sourceFilesList)
	sourceFilesList = slices.Compact(sourceFilesList)

	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	return webPkgIDs, sourceFilesList, nil
}

// NewImportBannerShim generates an esbuild require() shim for cjs module compatibility.
//
// This is a hack to work around issues with require() in esbuild.
// The require() function is not asynchronous but import() is.
// xfrmImport can be set to override the import path for a package.
//
// https://github.com/evanw/esbuild/issues/1921
func NewImportBannerShim(pkgs []string, minify bool, xfrmImport func(pkg string) string) string {
	var sb strings.Builder
	// write import statements
	// import * as __bldr_react from 'react';
	pkgVarNames := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		// clean pkg name for use as variable
		pkgVarName := strings.ReplaceAll(pkg, "@", "_")
		pkgVarName = strings.ReplaceAll(pkgVarName, "/", "_")
		pkgVarName = strings.ReplaceAll(pkgVarName, "-", "_")

		// prepend __bldr_ to variable name to deconflict
		pkgVarName = "__bldr_" + pkgVarName
		pkgVarNames[i] = pkgVarName

		_, _ = sb.WriteString("import * as ")
		_, _ = sb.WriteString(pkgVarName)
		_, _ = sb.WriteString(" from ")
		impPkg := pkg
		if xfrmImport != nil {
			impPkg = xfrmImport(impPkg)
			if impPkg == "" {
				impPkg = pkg
			}
		}
		_, _ = sb.WriteString(strconv.Quote(impPkg))
		_, _ = sb.WriteString(";\n")
	}

	// write require function implementation
	_, _ = sb.WriteString("const require = (pkgName) => {\n")
	_, _ = sb.WriteString("  switch (pkgName) {\n")
	for i, pkg := range pkgs {
		_, _ = sb.WriteString("  case ")
		_, _ = sb.WriteString(strconv.Quote(pkg))
		_, _ = sb.WriteString(":\n")
		_, _ = sb.WriteString("    return ")
		_, _ = sb.WriteString(pkgVarNames[i])
		_, _ = sb.WriteString(";\n")
	}
	_, _ = sb.WriteString("  default:\n")
	_, _ = sb.WriteString("    throw Error('Dynamic require of \"' + pkgName + '\" is not supported');\n")
	_, _ = sb.WriteString("  }\n};\n")

	// minify
	code := sb.String()
	result := esbuild_api.Transform(code, esbuild_api.TransformOptions{
		Target:    esbuild_api.ES2022,
		Sourcemap: esbuild_api.SourceMapNone,
		Platform:  esbuild_api.PlatformBrowser,

		MinifyWhitespace:  minify,
		MinifySyntax:      minify,
		MinifyIdentifiers: false,
	})
	return string(result.Code)
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
	opts.Banner["js"] = old + NewImportBannerShim(BldrExternal, opts.MinifySyntax, nil)
}
