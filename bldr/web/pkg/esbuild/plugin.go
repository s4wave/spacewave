package web_pkg_esbuild

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
				if ora.Importer == "bldr-pkg-resolve" {
					return result, nil
				}

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
				resolvePkgPath := func(path string, kind esbuild_api.ResolveKind) (string, error) {
					res := pb.Resolve(path, esbuild_api.ResolveOptions{
						Importer:  "bldr-pkg-resolve",
						Namespace: "file",
						// ResolveDir: projectRootDir, // ora.ResolveDir,
						ResolveDir: ora.ResolveDir,
						Kind:       kind,
					})
					if err := bldr_esbuild_build.ResolveResultToErr(res); err != nil {
						return "", err
					}
					// expect the path to have changed
					if res.Path == path || res.Path == "" {
						return "", errors.Errorf("web pkg %s import could not be resolved: %s", webPkgID, path)
					}
					return res.Path, nil
				}

				// First resolve the path to the root of the web pkg.
				resPkgRoot, err := resolvePkgPath(
					path.Join(webPkgID, "package.json"),
					esbuild_api.ResolveJSImportStatement,
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

				// Trim ./ and .
				relPkgSubPath = strings.TrimPrefix(relPkgSubPath, ".")
				relPkgSubPath = strings.TrimPrefix(relPkgSubPath, "/")

				// Adjust suffix if we will rewrite .js -> .mjs in the build process.
				relPkgSubImpPath := relPkgSubPath
				relPkgSubImpPathExt := filepath.Ext(relPkgSubPath)
				if isJSExtension(relPkgSubImpPathExt) {
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
