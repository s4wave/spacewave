//go:build !js

package web_pkg_vite

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"

	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	determine_cjs_exports "github.com/s4wave/spacewave/bldr/web/pkg/esbuild/determine-cjs-exports"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/pkg/errors"
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
	viteBundler bldr_vite.SRPCViteBundlerClient,
	cacheDir string,
) (webPkgIDs, sourcePaths []string, importMapEntries []ImportMapEntry, err error) {
	// Build list of web pkg IDs.
	for _, ref := range webPkgsRefs {
		webPkgIDs = append(webPkgIDs, ref.GetWebPkgId())
	}
	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	var sourceFilesList []string
	for _, webPkgRef := range webPkgsRefs {
		webPkgID := webPkgRef.GetWebPkgId()
		pkgOutputPath := filepath.Join(outputPath, webPkgID)

		// Build the sibling list: all web pkg IDs except the current one.
		siblingIDs := slices.DeleteFunc(slices.Clone(webPkgIDs), func(id string) bool {
			return id == webPkgID
		})

		le.
			WithField("web-pkg-id", webPkgID).
			WithField("web-pkg-imports", webPkgRef.GetImports()).
			Debug("building web pkg bundle with vite")

		// Generate ESM wrappers for CJS imports so Rolldown produces
		// named exports. Wrappers go in a temp dir under the output path.
		pkgRoot := webPkgRef.GetWebPkgRoot()
		imports := webPkgRef.GetImports()
		wrapperDir := filepath.Join(pkgOutputPath, ".cjs-wrappers")
		imports, wrapperErr := generateCjsWrappers(le, pkgRoot, imports, wrapperDir, isRelease)
		if wrapperErr != nil {
			return nil, nil, nil, errors.Wrapf(wrapperErr, "generate cjs wrappers for %s", webPkgID)
		}

		resp, err := viteBundler.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{
			PkgId:          webPkgID,
			PkgRoot:        pkgRoot,
			Imports:        imports,
			SiblingPkgIds:  siblingIDs,
			ExternalPkgs:   web_pkg_external.BldrExternal,
			OutDir:         pkgOutputPath,
			WebPkgBasePath: webPkgBasePath,
			IsRelease:      isRelease,
			CacheDir:       cacheDir,
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

		// Collect source files, making paths relative to codeRootPath.
		for _, srcFile := range resp.GetSourceFiles() {
			relPath, relErr := filepath.Rel(codeRootPath, srcFile)
			if relErr != nil {
				continue
			}
			sourceFilesList = append(sourceFilesList, relPath)
		}

		// Collect import map entries, prefixing output paths with the web pkg base path.
		for _, entry := range resp.GetImportMapEntries() {
			importMapEntries = append(importMapEntries, ImportMapEntry{
				Specifier:  entry.GetSpecifier(),
				OutputPath: path.Join(webPkgBasePath, webPkgID, entry.GetOutputPath()),
			})
		}
	}

	slices.Sort(sourceFilesList)
	sourceFilesList = slices.Compact(sourceFilesList)

	slices.Sort(webPkgIDs)
	webPkgIDs = slices.Compact(webPkgIDs)

	return webPkgIDs, sourceFilesList, importMapEntries, nil
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
) ([]string, error) {
	result := make([]string, len(imports))
	copy(result, imports)

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
		cjsResult, err := determine_cjs_exports.AnalyzeCjsExports(pkgRoot, "./"+imp, nil, nodeEnv)
		if err != nil {
			le.WithError(err).WithField("file", absPath).Debug("skipping cjs analysis")
			continue
		}
		if len(cjsResult.Exports) == 0 && !cjsResult.ExportDefault && cjsResult.Reexport == "" {
			continue
		}

		// If the module re-exports from another file (e.g. conditional
		// require based on NODE_ENV), resolve the reexport target and
		// point the wrapper at the resolved file directly. This avoids
		// Rolldown encountering require() calls in the conditional entry.
		wrapperImportPath := absPath
		if cjsResult.Reexport != "" {
			resolved, resolveErr := determine_cjs_exports.ResolveModuleWithNodePaths(
				filepath.Dir(absPath), cjsResult.Reexport, nil,
			)
			if resolveErr == nil {
				wrapperImportPath = resolved
				// Re-analyze the resolved file for its actual exports.
				resolvedResult, reErr := determine_cjs_exports.AnalyzeCjsExports(
					filepath.Dir(resolved), "./"+filepath.Base(resolved), nil, nodeEnv,
				)
				if reErr == nil && (len(resolvedResult.Exports) > 0 || resolvedResult.ExportDefault) {
					cjsResult = resolvedResult
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
			return nil, err
		}
		if err := os.WriteFile(wrapperPath, []byte(wrapperContent), 0o644); err != nil {
			return nil, err
		}

		le.WithFields(logrus.Fields{
			"file":    imp,
			"wrapper": wrapperPath,
			"exports": len(cjsResult.Exports),
		}).Debug("generated cjs esm wrapper")

		// Replace the import with the absolute wrapper path.
		result[i] = wrapperPath
	}

	return result, nil
}
