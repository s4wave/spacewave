package web_pkg_esbuild

import (
	"path"
	"path/filepath"
	"strings"

	util_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
	"github.com/evanw/esbuild/pkg/api"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

// BuildEsbuildPlugin constructs the bldr esbuild plugin.
//
// externalizePkgs will be marked as external and remapped to /b/pkg/{path}.
func BuildEsbuildPlugin(
	le *logrus.Entry,
	externalizePkgs []string,
	addWebPkgImport func(webPkgID, webPkgRoot, webPkgSubPath string),
) esbuild_api.Plugin {
	// add the bldr plugin
	// https://esbuild.github.io/plugins/#concepts
	return esbuild_api.Plugin{
		Name: "bldr",
		Setup: func(pb esbuild_api.PluginBuild) {
			pb.OnResolve(esbuild_api.OnResolveOptions{
				Filter: ".",
				// Filter: "^example/$",
				Namespace: "file",
			}, func(ora esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				var result esbuild_api.OnResolveResult

				// If the import path has a valid web pkg id prefix, use it.
				// Otherwise ignore this import.
				webPkgID, webPkgSubPath, err := web_pkg.CheckStripWebPkgIdPrefix(ora.Path)
				if err != nil {
					return result, nil
				}

				// Match the import to the configured list of externalized imports.
				// Objective: we want to import a singleton instance of modules like React.
				// Mark the import as external so we will import() it dynamically at runtime.
				//
				// Add the original import path to the list of embedded external modules.
				// Transform the import path to the new import path: /bldr/pkg/...
				// Later: include the list of embedded external modules in the assets fs.
				// Later: add an import map with the list of modules + paths to import from.
				// "react" -> /p/web/pkg/react
				if !slices.Contains(externalizePkgs, webPkgID) {
					return result, nil
				}

				// Use esbuild's resolution algorithm.
				resolvePkgPath := func(path string, kind api.ResolveKind) (string, error) {
					res := pb.Resolve(path, esbuild_api.ResolveOptions{
						Kind:       kind,
						ResolveDir: ora.ResolveDir,
						Importer:   ora.Importer,
						Namespace:  "bldr-pkg-resolve",
					})
					if err := util_esbuild.ResolveResultToErr(res); err != nil {
						return "", err
					}
					return res.Path, nil
				}

				// First resolve the path to the root of the web pkg.
				resPkgRoot, err := resolvePkgPath(
					path.Join(webPkgID, "package.json"),
					api.ResolveJSImportStatement,
				)
				if err != nil {
					return result, err
				}
				resPkgRoot = filepath.Dir(resPkgRoot)

				// Rewrite the import path to be more specific, if necessary:
				// e.g. "react-dom/client" -> react-dom/client.js according to "exports" in package.json
				resPkgSubPath, err := resolvePkgPath(ora.Path, ora.Kind)
				if err != nil {
					return result, err
				}

				// Expect that the pkg sub-path is a sub-dir of resPkgRoot.
				relPkgSubPath, err := filepath.Rel(resPkgRoot, resPkgSubPath)
				if err != nil {
					return result, err
				}

				if strings.HasPrefix(relPkgSubPath, "..") {
					return result, errors.Errorf(
						"web pkg %s import %s resolved to path outside pkg dir %s: %s",
						webPkgID,
						webPkgSubPath,
						resPkgRoot,
						relPkgSubPath,
					)
				}

				// Trim ./
				relPkgSubPath = strings.TrimPrefix(relPkgSubPath, "./")

				// Adjust suffix if we will rewrite .js -> .mjs in the build process.
				relPkgSubImpPath := relPkgSubPath
				relPkgSubImpPathExt := filepath.Ext(relPkgSubPath)
				if determine_cjs_exports.SupportsExtension(relPkgSubImpPathExt) {
					// remap the output file extension to .mjs
					relPkgSubImpPath = relPkgSubImpPath[:len(relPkgSubPath)-len(relPkgSubImpPathExt)] + ".mjs"
				}

				// Remap import path and namespace.
				result.Namespace = "bldr"
				result.Path = path.Join("/b/pkg", webPkgID, relPkgSubImpPath)

				// Mark package as external so esbuild uses import(path) to load it.
				result.External = true

				// Mark the import path we will need to bundle later.
				if addWebPkgImport != nil {
					addWebPkgImport(webPkgID, resPkgRoot, relPkgSubPath)
				}

				return result, nil
			})
		},
	}
}

// MergeEsbuildBuildOpts merges esbuild build options.
func MergeEsbuildBuildOpts(target, source *esbuild_api.BuildOptions) {
	mergeValueIfSet(&target.Target, source.Target)
	if len(source.Engines) != 0 {
		target.Engines = source.Engines
	}
	mergeValueIfSet(&target.LogLevel, source.LogLevel)
	mergeValueIfSet(&target.LogLimit, source.LogLimit)
	mergeMapOverwrite(&target.LogOverride, source.LogOverride)
	mergeMapOverwrite(&target.Supported, source.Supported)
	mergeValueIfSet(&target.MangleProps, source.MangleProps)
	mergeValueIfSet(&target.ReserveProps, source.ReserveProps)
	mergeValueIfSet(&target.MangleQuoted, source.MangleQuoted)
	mergeValueIfSet(&target.Drop, source.Drop)
	mergeValueIfSet(&target.TreeShaking, source.TreeShaking)
	mergeValueIfSet(&target.IgnoreAnnotations, source.IgnoreAnnotations)
	mergeValueIfSet(&target.LegalComments, source.LegalComments)
	mergeValueIfSet(&target.JSX, source.JSX)
	mergeValueIfSet(&target.JSXFactory, source.JSXFactory)
	mergeValueIfSet(&target.JSXImportSource, source.JSXImportSource)
	mergeValueIfSet(&target.JSXDev, source.JSXDev)
	mergeValueIfSet(&target.JSXSideEffects, source.JSXSideEffects)
	mergeMapOverwrite(&target.Define, source.Define)
	if len(source.Pure) != 0 {
		target.Pure = append(target.Pure, source.Pure...)
	}
	mergeValueIfSet(&target.KeepNames, source.KeepNames)
	mergeValueIfSet(&target.Platform, source.Platform)
	if len(source.External) != 0 {
		target.External = append(target.External, source.External...)
	}
	mergeValueIfSet(&target.Packages, source.Packages)
	mergeMapOverwrite(&target.Alias, source.Alias)
	if len(source.MainFields) != 0 {
		target.MainFields = append(target.MainFields, source.MainFields...)
	}
	if len(source.Conditions) != 0 {
		target.Conditions = append(target.Conditions, source.Conditions...)
	}
	mergeMapOverwrite(&target.Loader, source.Loader)
	if len(source.ResolveExtensions) != 0 {
		target.ResolveExtensions = append(target.ResolveExtensions, source.ResolveExtensions...)
	}
	mergeValueIfSet(&target.Tsconfig, source.Tsconfig)
	mergeMapOverwrite(&target.OutExtension, source.OutExtension)
	if len(source.Inject) != 0 {
		target.Inject = append(target.Inject, source.Inject...)
	}
	mergeMapOverwrite(&target.Banner, source.Banner)
	mergeMapOverwrite(&target.Footer, source.Footer)
	mergeValueIfSet(&target.EntryNames, source.EntryNames)
	mergeValueIfSet(&target.ChunkNames, source.ChunkNames)
	mergeValueIfSet(&target.AssetNames, source.AssetNames)
	if len(source.Plugins) != 0 {
		target.Plugins = append(target.Plugins, source.Plugins...)
	}
}
