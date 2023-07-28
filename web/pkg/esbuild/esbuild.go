package web_pkg_esbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
	determine_cjs_exports_exec "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports/exec"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

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
	webPkgsRefs []*WebPkgRef,
	outputPath string,
	webPkgBasePath string,
	isRelease bool,
) (webPkgIDs, sourcePaths []string, err error) {
	// TODO: Externalize + add imports for any imports within externalized packages.
	// This would require repeatedly calling esbuild until we discover all imports for each package.
	// Since this is significantly more complex, it's been left out for now.
	//
	// Example case: a package we externalize in the web_pkgs list imports
	// React: we would want to replace the imports to "react" with
	// /b/pkg/react/... as well.

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
	for _, webPkgRef := range webPkgsRefs {
		webPkgID := webPkgRef.WebPkgID
		webPkgIDs = append(webPkgIDs, webPkgID)

		/*
			webPkgRoot, err := filepath.Rel(codeRootPath, webPkgRef.WebPkgRoot)
			if err != nil {
				return nil, nil, err
			}
		*/

		pkgOutputPath := filepath.Join(outputPath, webPkgID)
		if _, err := os.Stat(pkgOutputPath); !os.IsNotExist(err) {
			if err := os.RemoveAll(pkgOutputPath); err != nil {
				return nil, nil, err
			}
		}

		pkgPublicPath := path.Join(webPkgBasePath, webPkgID)

		// Create a temporary dir for the entrypoints
		pkgBuildPath := webPkgRef.WebPkgRoot
		pkgTmpPath := filepath.Join(pkgOutputPath, "__bldr_esbuild_tmp")
		if err := os.MkdirAll(pkgTmpPath, 0755); err != nil {
			return nil, nil, err
		}

		// Determine the list of exports for each of the imports.
		buildEntrypoints := make([]esbuild_api.EntryPoint, len(webPkgRef.Imports))
		origEntrypointImpPaths := make(map[string]string)
		pkgRootAlias := "@bldr-web-pkg"
		for i, impPath := range webPkgRef.Imports {
			// webPkgImpPath := filepath.Join(webPkgRoot, impPath)
			webPkgImpPath := path.Join(webPkgID, impPath)
			impOutPath := filepath.Join(pkgOutputPath, impPath)

			// we should only process js-like files
			// pass other imports directly to esbuild as-is
			ext := filepath.Ext(webPkgImpPath)
			if !determine_cjs_exports.SupportsExtension(ext) {
				buildEntrypoints[i] = esbuild_api.EntryPoint{
					InputPath:  webPkgImpPath,
					OutputPath: impOutPath,
				}
				continue
			}

			webPkgExports, err := determine_cjs_exports_exec.ExecDetermineCjsExports(
				ctx,
				le.WithFields(logrus.Fields{
					"exec":        "determine-cjs-exports",
					"web-pkg-id":  webPkgID,
					"web-pkg-imp": impPath,
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
			if err := os.MkdirAll(outEntrypointDir, 0755); err != nil {
				return nil, nil, err
			}

			err = os.WriteFile(outEntrypointPath, []byte(webPkgEntrypointScript+"\n"), 0644)
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
		buildOpts.Sourcemap = esbuild_api.SourceMapNone
		buildOpts.TreeShaking = esbuild_api.TreeShakingFalse
		buildOpts.OutExtension = map[string]string{".js": ".mjs"}
		buildOpts.Alias[pkgRootAlias] = webPkgRef.WebPkgRoot
		buildOpts.Alias[webPkgID] = webPkgRef.WebPkgRoot

		// add banner
		msg := fmt.Sprintf("built by bldr/web/pkg: %s/%v", webPkgID, webPkgRef.Imports)
		buildOpts.Banner["js"] = "// " + msg
		buildOpts.Banner["css"] = "/* " + msg + " */"

		le.Debugf("compiling web pkg bundle with esbuild: %s", webPkgID)
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

		metaFile := &bldr_esbuild.EsbuildMetafile{}
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
