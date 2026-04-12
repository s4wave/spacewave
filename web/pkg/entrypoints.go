//go:build !js

package web_pkg

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// tsExtensions are the file extensions to try when resolving an entry point.
var tsExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}

// indexFiles are the index file names to try when resolving a directory entry point.
var indexFiles = []string{
	"index.ts", "index.tsx", "index.js", "index.jsx", "index.mjs",
}

// WebPkgEntrypointConfig is the subset of entrypoint config needed for resolution.
// Avoids importing web/bundler proto types to prevent import cycles.
type WebPkgEntrypointConfig struct {
	// Path is the subpath export specifier (e.g. ".", "./object").
	Path string
}

// ResolveWebPkgEntrypoints resolves the entry points for a web package
// into file paths relative to pkgRoot.
//
// For project-local packages (no package.json with exports):
//   - Uses configured entrypoints, or defaults to ["."].
//   - "." resolves to the root index file.
//   - "./foo" resolves to foo.ts, foo.tsx, foo/index.ts, etc.
//
// For node_modules packages (has package.json with exports or main):
//   - Uses configured entrypoints if set.
//   - Otherwise reads package.json exports field to discover entry points.
//   - Falls back to main field or index.js.
func ResolveWebPkgEntrypoints(
	pkgRoot string,
	entrypoints []WebPkgEntrypointConfig,
) ([]string, error) {
	// Check if this is a node_modules package by looking for package.json.
	pkgJsonPath := filepath.Join(pkgRoot, "package.json")
	pkgJsonData, pkgJsonErr := os.ReadFile(pkgJsonPath)
	isNodeModule := pkgJsonErr == nil && len(pkgJsonData) > 0

	if isNodeModule {
		return resolveNodeModuleEntrypoints(pkgRoot, pkgJsonData, entrypoints)
	}
	return resolveLocalEntrypoints(pkgRoot, entrypoints)
}

// resolveLocalEntrypoints resolves entry points for a project-local package.
func resolveLocalEntrypoints(
	pkgRoot string,
	entrypoints []WebPkgEntrypointConfig,
) ([]string, error) {
	// Default to root index if no entrypoints configured.
	if len(entrypoints) == 0 {
		entrypoints = []WebPkgEntrypointConfig{{Path: "."}}
	}

	var imports []string
	for _, ep := range entrypoints {
		subpath := ep.Path
		if subpath == "" {
			subpath = "."
		}

		resolved, err := resolveSubpathEntrypoints(pkgRoot, subpath)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve entrypoint %q", subpath)
		}
		imports = append(imports, resolved...)
	}
	return imports, nil
}

// resolveNodeModuleEntrypoints resolves entry points for a node_modules package.
func resolveNodeModuleEntrypoints(
	pkgRoot string,
	pkgJsonData []byte,
	entrypoints []WebPkgEntrypointConfig,
) ([]string, error) {
	// If explicit entrypoints are configured, use those.
	if len(entrypoints) > 0 {
		return resolveLocalEntrypoints(pkgRoot, entrypoints)
	}

	// Parse package.json to find exports or main.
	var p fastjson.Parser
	v, err := p.ParseBytes(pkgJsonData)
	if err != nil {
		return nil, errors.Wrap(err, "parse package.json")
	}

	// Try exports field first.
	if exports := v.Get("exports"); exports != nil {
		imports := resolvePackageJSONExports(exports)
		if len(imports) != 0 {
			return imports, nil
		}
	}

	// Fall back to main or module field.
	for _, entryBytes := range [][]byte{v.GetStringBytes("module"), v.GetStringBytes("main")} {
		entry := string(entryBytes)
		if entry == "" {
			continue
		}
		ext := filepath.Ext(entry)
		switch ext {
		case ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".css":
			return []string{entry}, nil
		}
	}

	// No JS exports/main/module: treat as local package for entrypoint resolution.
	return resolveLocalEntrypoints(pkgRoot, nil)
}

// resolvePackageJSONExports extracts entry points from a package.json exports value.
//
// Handles the common patterns:
//
//	{ ".": "./dist/index.mjs" }
//	{ ".": { "import": "./dist/index.mjs" } }
//	{ ".": { "import": { "default": "./dist/index.mjs" } } }
//	{ "./jsx-runtime": { "import": { "default": "./jsx-runtime.js" } } }
//
// Skips entries that resolve to non-bundleable files (types, binary).
func resolvePackageJSONExports(exports *fastjson.Value) []string {
	if exports == nil {
		return nil
	}
	if resolved := resolveExportCondition(exports); resolved != "" {
		if importPath, ok := normalizeResolvedExport(resolved); ok {
			return []string{importPath}
		}
		return []string{"index.js"}
	}

	var imports []string
	obj := exports.GetObject()
	if obj == nil {
		return []string{"index.js"}
	}
	obj.Visit(func(k []byte, raw *fastjson.Value) {
		subpath := string(k)
		// Skip internal/private exports.
		if strings.HasPrefix(subpath, "#") {
			return
		}

		// Skip wildcard exports (e.g. "./*", "./*.css", "./files/*").
		if strings.Contains(subpath, "*") {
			return
		}

		resolved := resolveExportCondition(raw)
		if resolved == "" {
			return
		}

		importPath, ok := normalizeResolvedExport(resolved)
		if !ok {
			return
		}
		imports = append(imports, importPath)
	})

	if len(imports) == 0 {
		return []string{"index.js"}
	}
	return imports
}

// resolveExportCondition resolves a package.json export value to a file path.
// Handles string values and nested condition objects (import > default > require).
func resolveExportCondition(raw *fastjson.Value) string {
	if raw == nil {
		return ""
	}

	// Try string first.
	if s := string(raw.GetStringBytes()); s != "" {
		return s
	}

	// Try condition object.
	obj := raw.GetObject()
	if obj == nil {
		return ""
	}

	// Prefer import > default > require.
	for _, key := range []string{"import", "default", "require"} {
		if result := resolveExportCondition(raw.Get(key)); result != "" {
			return result
		}
	}

	// If no standard condition matched, try the first value.
	first := ""
	obj.Visit(func(_ []byte, val *fastjson.Value) {
		if first == "" {
			first = resolveExportCondition(val)
		}
	})
	return first
}

func normalizeResolvedExport(resolved string) (string, bool) {
	// Skip wildcard resolved paths.
	if strings.Contains(resolved, "*") {
		return "", false
	}

	// Skip non-bundleable exports (types, binary, license).
	ext := filepath.Ext(resolved)
	switch ext {
	case ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".css":
	default:
		return "", false
	}

	return strings.TrimPrefix(resolved, "./"), true
}

// resolveSubpathEntrypoints resolves a subpath specifier (like ".", "./object")
// to one or more file paths relative to pkgRoot.
//
// Resolution order:
//  1. Direct file match (with extensions): "./foo" -> "foo.ts"
//  2. Directory with index file: "./foo" -> "foo/index.ts"
//  3. Directory scan: "./foo" -> all .ts/.tsx files in foo/
//
// Step 3 allows directory-level entrypoints to work without barrel files.
func resolveSubpathEntrypoints(pkgRoot, subpath string) ([]string, error) {
	// Normalize: strip leading "./"
	rel := strings.TrimPrefix(subpath, "./")
	if rel == "." || rel == "" {
		rel = ""
	}

	// Try as a direct file first (with extensions).
	if rel != "" {
		for _, ext := range tsExtensions {
			candidate := rel + ext
			if fileExists(filepath.Join(pkgRoot, candidate)) {
				return []string{candidate}, nil
			}
		}
	}

	// Try as a directory with index files.
	dir := rel
	for _, idx := range indexFiles {
		candidate := filepath.Join(dir, idx)
		if fileExists(filepath.Join(pkgRoot, candidate)) {
			return []string{candidate}, nil
		}
	}

	// If rel already has an extension and the file exists, use it directly.
	if rel != "" && filepath.Ext(rel) != "" {
		if fileExists(filepath.Join(pkgRoot, rel)) {
			return []string{rel}, nil
		}
	}

	// Try scanning the directory for all TS/TSX/JS files.
	absDir := filepath.Join(pkgRoot, dir)
	if info, err := os.Stat(absDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(absDir)
		if err != nil {
			return nil, errors.Wrapf(err, "read directory %q", subpath)
		}
		var files []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			switch ext {
			case ".ts", ".tsx", ".js", ".jsx", ".mjs":
				// Skip test files and declaration files.
				name := entry.Name()
				if strings.HasSuffix(name, ".test.ts") ||
					strings.HasSuffix(name, ".test.tsx") ||
					strings.HasSuffix(name, ".d.ts") {
					continue
				}
				files = append(files, filepath.Join(dir, name))
			}
		}
		if len(files) > 0 {
			return files, nil
		}
	}

	return nil, errors.Errorf("could not resolve entry point %q in %s", subpath, pkgRoot)
}

// ResolveWebPkgRefsFromConfig builds WebPkgRef entries from config and
// Vite-discovered roots. Entry points are resolved from config (entrypoints
// field or package.json exports) rather than regex-discovered subpaths.
//
// viteRefs provides package roots discovered by the Vite plugin during the
// main build. These are used to determine the filesystem root for each
// package. Packages not found in viteRefs are resolved via node_modules.
//
// configEntrypoints maps web pkg ID to its entrypoint configs (from
// WebPkgRefConfig.Entrypoints). Pass nil for packages that should use defaults.
func ResolveWebPkgRefsFromConfig(
	codeRootPath string,
	pkgConfigs []WebPkgResolveConfig,
	viteRefs []*WebPkgRef,
	excludedIDs map[string]struct{},
) ([]*WebPkgRef, error) {
	// Build a lookup of roots from the Vite build.
	rootsByID := make(map[string]string, len(viteRefs))
	for _, ref := range viteRefs {
		rootsByID[ref.GetWebPkgId()] = ref.GetWebPkgRoot()
	}

	var refs []*WebPkgRef
	for _, conf := range pkgConfigs {
		pkgID := conf.ID
		if pkgID == "" || conf.Exclude {
			continue
		}
		if _, excluded := excludedIDs[pkgID]; excluded {
			continue
		}

		// Determine package root.
		pkgRoot := rootsByID[pkgID]
		if pkgRoot == "" {
			// Try node_modules fallback.
			candidate := filepath.Join(codeRootPath, "node_modules", pkgID)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				pkgRoot = candidate
			}
		}
		if pkgRoot == "" {
			continue
		}

		// Resolve entry points from config.
		imports, err := ResolveWebPkgEntrypoints(pkgRoot, conf.Entrypoints)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve entry points for web pkg %s", pkgID)
		}
		if len(imports) == 0 {
			continue
		}

		refs = append(refs, &WebPkgRef{
			WebPkgId:   pkgID,
			WebPkgRoot: pkgRoot,
			Imports:    imports,
		})
	}

	SortWebPkgRefs(refs)
	return refs, nil
}

// WebPkgResolveConfig holds the config needed to resolve a web package.
// Avoids importing web/bundler proto types to prevent import cycles.
type WebPkgResolveConfig struct {
	ID          string
	Exclude     bool
	Entrypoints []WebPkgEntrypointConfig
}

// fileExists reports whether the named file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
