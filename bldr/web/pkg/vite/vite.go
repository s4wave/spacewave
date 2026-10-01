//go:build !js

package web_pkg_vite

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/pkg/errors"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	determine_cjs_exports "github.com/s4wave/spacewave/bldr/web/pkg/esbuild/determine-cjs-exports"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/sirupsen/logrus"
)

// ImportMapEntry is an entry mapping a logical import specifier to a hashed output path.
type ImportMapEntry struct {
	// Specifier is the logical import specifier (e.g. "react", "react/jsx-runtime").
	Specifier string
	// OutputPath is the hashed output filename (e.g. "index-a1b2c3.mjs").
	OutputPath string
}

// BuildWebPkgsVite builds web packages using the ViteBundler SRPC service.
//
// Has the same return signature as BuildWebPkgsEsbuild: web pkg IDs, source file
// paths, and an error. Additionally returns import map entries mapping logical
// specifiers to hashed output filenames.
func BuildWebPkgsVite(
	ctx context.Context,
	le *logrus.Entry,
	codeRootPath string,
	webPkgsRefs []*web_pkg.WebPkgRef,
	outputPath string,
	webPkgBasePath string,
	isRelease bool,
	jsMinification bool,
	jsSourcemaps bool,
	viteBundler bldr_vite.SRPCViteBundlerClient,
	cacheDir string,
) (webPkgIDs, sourcePaths []string, importMapEntries []ImportMapEntry, err error) {
	return BuildWebPkgsViteWithManagedRoot(
		ctx, le, codeRootPath, "", webPkgsRefs, nil, outputPath, webPkgBasePath,
		isRelease, jsMinification, jsSourcemaps, viteBundler, cacheDir,
	)
}

// BuildWebPkgsViteWithManagedRoot builds web packages without retaining
// compiler-managed files as durable source provenance.
//
// providedWebPkgIDs lists web packages other plugins provide. Imports of them
// stay external and resolve to their /b/pkg/ URLs, like sibling packages.
func BuildWebPkgsViteWithManagedRoot(
	ctx context.Context,
	le *logrus.Entry,
	codeRootPath string,
	managedRootPath string,
	webPkgsRefs []*web_pkg.WebPkgRef,
	providedWebPkgIDs []string,
	outputPath string,
	webPkgBasePath string,
	isRelease bool,
	jsMinification bool,
	jsSourcemaps bool,
	viteBundler bldr_vite.SRPCViteBundlerClient,
	cacheDir string,
) (webPkgIDs, sourcePaths []string, importMapEntries []ImportMapEntry, err error) {
	// Canonicalize the code root and prepare the managed source root.
	canonicalCodeRoot, err := filepath.Abs(codeRootPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if resolvedRoot, resolveErr := filepath.EvalSymlinks(canonicalCodeRoot); resolveErr == nil {
		canonicalCodeRoot = resolvedRoot
	}
	var managedRoot *managedSourceRoot
	if managedRootPath != "" {
		managedRoot, err = newManagedSourceRoot(canonicalCodeRoot, managedRootPath)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Build the deduplicated list of web pkg IDs.
	for _, ref := range webPkgsRefs {
		webPkgIDs = append(webPkgIDs, ref.GetWebPkgId())
	}
	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	// Build each web pkg bundle with Vite and collect its provenance.
	var sourceFilesList []string
	for _, webPkgRef := range webPkgsRefs {
		webPkgID := webPkgRef.GetWebPkgId()
		pkgOutputPath := filepath.Join(outputPath, webPkgID)

		// Build the sibling list: all web pkg IDs except the current one, plus
		// the packages other plugins provide.
		siblingIDs := slices.DeleteFunc(slices.Clone(webPkgIDs), func(id string) bool {
			return id == webPkgID
		})
		siblingIDs = append(siblingIDs, providedWebPkgIDs...)
		externalIDs := slices.DeleteFunc(slices.Clone(web_pkg_external.BldrExternal), func(id string) bool {
			return id == webPkgID
		})

		le.
			WithField("web-pkg-id", webPkgID).
			WithField("web-pkg-imports", webPkgRef.GetImports()).
			Debug("building web pkg bundle with vite")

		// Generate ESM wrappers for CJS imports so Rolldown produces
		// named exports. Keep wrappers in the build cache: Vite empties OutDir
		// before building, and the wrappers embed absolute paths, so they must
		// not ship in the output.
		pkgRoot := webPkgRef.GetWebPkgRoot()
		imports := webPkgRef.GetImports()
		wrapperDir := filepath.Join(cacheDir, ".cjs-wrappers")
		imports, wrapperSources, generatedWrappers, wrapperErr := generateCjsWrappers(le, pkgRoot, imports, wrapperDir, isRelease)
		if wrapperErr != nil {
			return nil, nil, nil, errors.Wrapf(wrapperErr, "generate cjs wrappers for %s", webPkgID)
		}

		resp, err := viteBundler.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{
			PkgId:          webPkgID,
			PkgRoot:        pkgRoot,
			Imports:        imports,
			SiblingPkgIds:  siblingIDs,
			ExternalPkgs:   externalIDs,
			OutDir:         pkgOutputPath,
			WebPkgBasePath: webPkgBasePath,
			IsRelease:      isRelease,
			CacheDir:       cacheDir,
			JsMinification: jsMinification,
			JsSourcemaps:   jsSourcemaps,
		})
		if ctx.Err() != nil {
			return nil, nil, nil, context.Canceled
		}
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "build web pkg %s", webPkgID)
		}
		if !resp.GetSuccess() {
			return nil, nil, nil, errors.Errorf("vite build web pkg %s failed: %s", webPkgID, resp.GetError())
		}

		if err := removeViteManifest(pkgOutputPath); err != nil {
			return nil, nil, nil, errors.Wrapf(err, "remove vite manifest for %s", webPkgID)
		}

		// Collect stable source inputs without recording generated CJS wrappers.
		// Wrapper provenance remains cache input even when Vite reports only the
		// generated module that imported it.
		for _, srcFile := range append(resp.GetSourceFiles(), wrapperSources...) {
			canonicalPath, pathErr := canonicalSourcePath(canonicalCodeRoot, srcFile)
			if pathErr != nil {
				continue
			}
			if _, generated := generatedWrappers[canonicalPath]; generated {
				continue
			}
			relPath, relErr := filepath.Rel(canonicalCodeRoot, canonicalPath)
			if relErr != nil {
				continue
			}
			if managedRoot != nil && managedRoot.Contains(canonicalPath) {
				continue
			}
			sourceFilesList = append(sourceFilesList, filepath.ToSlash(relPath))
		}

		// Collect import map entries, prefixing output paths with the web pkg base path.
		for _, entry := range resp.GetImportMapEntries() {
			importMapEntries = append(importMapEntries, ImportMapEntry{
				Specifier:  entry.GetSpecifier(),
				OutputPath: path.Join(webPkgBasePath, webPkgID, entry.GetOutputPath()),
			})
		}
	}

	// Return the deduplicated web pkg IDs and source paths.
	slices.Sort(sourceFilesList)
	sourceFilesList = slices.Compact(sourceFilesList)

	// Deduplicate the web pkg ID list.
	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	return webPkgIDs, sourceFilesList, importMapEntries, nil
}

// removeViteManifest deletes the build-time Vite manifest from a web pkg
// output. Its keys are paths from the build directory to the generated CJS
// wrappers, so it differs between machines, and the runtime resolves web pkgs
// through the import map instead. It leaves the rest of the .vite directory,
// which holds debug output when enabled.
func removeViteManifest(pkgOutputPath string) error {
	// Delete the manifest, tolerating a missing file.
	viteDir := filepath.Join(pkgOutputPath, ".vite")
	err := os.Remove(filepath.Join(viteDir, "manifest.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove the directory only when the manifest was its last file.
	_ = os.Remove(viteDir)
	return nil
}

func canonicalSourcePath(rootPath, sourcePath string) (string, error) {
	// Canonicalize the root path, following symlinks when possible.
	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	if canonicalRoot, canonicalErr := filepath.EvalSymlinks(rootAbs); canonicalErr == nil {
		rootAbs = canonicalRoot
	}

	// Canonicalize the source path against the resolved root.
	path := sourcePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootAbs, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if canonicalPath, canonicalErr := filepath.EvalSymlinks(path); canonicalErr == nil {
		path = canonicalPath
	}
	return path, nil
}

// generateCjsWrappers analyzes each import file and generates ESM wrappers
// for CJS modules. Returns a new imports list where CJS entries point to
// wrapper .mjs files with named re-exports.
func generateCjsWrappers(
	le *logrus.Entry,
	pkgRoot string,
	imports []string,
	wrapperDir string,
	isRelease bool,
) ([]string, []string, map[string]struct{}, error) {
	// Start from the original imports and track wrapper provenance.
	result := make([]string, len(imports))
	var sources []string
	generated := make(map[string]struct{})
	copy(result, imports)

	// Analyze each import and replace CJS modules with generated wrappers.
	for i, imp := range imports {
		ext := filepath.Ext(imp)
		if !determine_cjs_exports.SupportsExtension(ext) {
			continue
		}

		absPath := filepath.Join(pkgRoot, imp)
		nodeEnv := "development"
		if isRelease {
			nodeEnv = "production"
		}
		cjsResult, analysisSources, err := determine_cjs_exports.AnalyzeCjsExportsWithProvenance(pkgRoot, "./"+imp, nil, nodeEnv)
		if err != nil {
			le.WithError(err).WithField("file", absPath).Debug("skipping cjs analysis")
			continue
		}
		sources = append(sources, analysisSources...)
		if len(cjsResult.Exports) == 0 && !cjsResult.ExportDefault && cjsResult.Reexport == "" {
			continue
		}

		// If the module re-exports from another file (e.g. conditional
		// require based on NODE_ENV), resolve the reexport target and
		// point the wrapper at the resolved file directly. This avoids
		// Rolldown encountering require() calls in the conditional entry.
		wrapperImportPath := absPath
		if cjsResult.Reexport != "" {
			resolved, resolutionFiles, resolveErr := determine_cjs_exports.ResolveModuleWithProvenance(
				filepath.Dir(absPath), cjsResult.Reexport, nil,
			)
			sources = append(sources, resolutionFiles...)
			if resolveErr == nil {
				wrapperImportPath = resolved
				// Re-analyze the resolved file for its actual exports.
				resolvedResult, resolvedSources, reErr := determine_cjs_exports.AnalyzeCjsExportsWithProvenance(
					filepath.Dir(resolved), "./"+filepath.Base(resolved), nil, nodeEnv,
				)
				if reErr == nil {
					sources = append(sources, resolvedSources...)
					if len(resolvedResult.Exports) > 0 || resolvedResult.ExportDefault {
						cjsResult = resolvedResult
					}
				}
			}
		}

		// Generate ESM wrapper that re-exports the CJS named exports.
		wrapperContent := determine_cjs_exports.GenerateRemapExports(wrapperImportPath, cjsResult)

		// Write wrapper to temp dir, preserving the import sub-path structure.
		wrapperPath := filepath.Join(wrapperDir, imp)
		wrapperExt := filepath.Ext(wrapperPath)
		if wrapperExt == ".js" || wrapperExt == ".cjs" {
			wrapperPath = wrapperPath[:len(wrapperPath)-len(wrapperExt)] + ".mjs"
		}

		if err := os.MkdirAll(filepath.Dir(wrapperPath), 0o755); err != nil {
			return nil, nil, nil, err
		}
		if err := os.WriteFile(wrapperPath, []byte(wrapperContent), 0o644); err != nil {
			return nil, nil, nil, err
		}
		canonicalWrapper, err := canonicalSourcePath("", wrapperPath)
		if err != nil {
			return nil, nil, nil, err
		}
		wrapperPath = canonicalWrapper
		generated[wrapperPath] = struct{}{}

		le.WithFields(logrus.Fields{
			"file":    imp,
			"wrapper": wrapperPath,
			"exports": len(cjsResult.Exports),
		}).Debug("generated cjs esm wrapper")

		// Replace the import with the absolute wrapper path.
		result[i] = wrapperPath
	}

	return result, sources, generated, nil
}
