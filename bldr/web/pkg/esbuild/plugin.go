//go:build !js

package web_pkg_esbuild

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/sirupsen/logrus"
)

// BuildEsbuildPlugin constructs the bldr esbuild plugin.
//
// Imports of externalizePkgs are marked external and rewritten to the bare
// specifier web_pkg.ImportSpecifierPrefix + "{id}/{path}", which the document's
// import map resolves to the shared module the host loaded.
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
				// Initialize a result for imports that this plugin leaves unresolved.
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

				// Externalize only the configured packages, so the page holds
				// one instance of modules like React.
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

				// Reject package imports that resolve outside the selected package root.
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

				// Adjust the suffix if the build process
				// rewrites .js -> .mjs.
				relPkgSubImpPath := relPkgSubPath
				relPkgSubImpPathExt := filepath.Ext(relPkgSubPath)
				if isJSExtension(relPkgSubImpPathExt) {
					// remap the output file extension to .mjs
					relPkgSubImpPath = relPkgSubImpPath[:len(relPkgSubPath)-len(relPkgSubImpPathExt)] + ".mjs"
				}

				// Import the shared module through the document's import map.
				result.Namespace = "bldr"
				result.Path = web_pkg.ImportSpecifierPrefix + path.Join(webPkgID, relPkgSubImpPath)
				result.External = true

				// Mark the import path the bundler will need
				// to bundle later.
				if addWebPkgImport != nil {
					addWebPkgImport(webPkgID, resPkgRoot, relPkgSubPath)
				}

				return result, nil
			})
		},
	}
}
