package esbuild

import (
	"context"
	"crypto/sha256"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/project/bunlock"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Modules resolves browser modules from source and verified dependency blocks.
// Its context and borrowed inputs remain valid until the esbuild build ends.
// Expanded packages and input provenance live only for that build.
type Modules struct {
	// ctx cancels reads during this build.
	ctx context.Context
	// source is the caller's immutable source snapshot.
	source fs.FS
	// dependencies contains tarballs verified by bunlock.Fetcher.
	dependencies kvtx.TxOps
	// mtx serializes archive expansion and provenance updates across callbacks.
	mtx csync.Mutex
	// packages maps virtual node_modules roots to integrity-pinned packages.
	packages map[string]bunlock.Package
	// archives holds expanded package files by integrity for this build only.
	archives map[string]map[string][]byte
	// remaining bounds uncompressed package bytes across the whole build.
	remaining int64
	// external contains host-provided package IDs.
	external []string
	// inputs records source files read during module resolution and compilation.
	inputs map[string]*builder.InputManifest_File
}

// NewModules constructs one build's resolver. dependencies must be populated
// only with verified tarballs, as required by bunlock.Fetcher.Fetch.
// maxExpandedSize must be positive and smaller than math.MaxInt64.
func NewModules(ctx context.Context, source fs.FS, dependencies kvtx.TxOps, packages []bunlock.Package, maxExpandedSize int64, external []string) *Modules {
	// Map Bun's nested install keys to Node's nested node_modules directories.
	pins := make(map[string]bunlock.Package, len(packages))
	for _, pkg := range packages {
		parts := strings.Split(pkg.Key, "/")
		var install strings.Builder
		install.WriteString("project")
		for i := 0; i < len(parts); i++ {
			install.WriteString("/node_modules/")
			install.WriteString(parts[i])
			if strings.HasPrefix(parts[i], "@") && i+1 < len(parts) {
				i++
				install.WriteString("/")
				install.WriteString(parts[i])
			}
		}
		pins[install.String()] = pkg
	}

	// Retain caller inputs while keeping expansion and provenance local to the build.
	return &Modules{
		ctx: ctx, source: source, dependencies: dependencies,
		packages: pins, archives: make(map[string]map[string][]byte),
		remaining: maxExpandedSize, external: slices.Clone(external),
		inputs: make(map[string]*builder.InputManifest_File),
	}
}

// Plugin supplies esbuild's reads and resolution without falling through to disk.
func (m *Modules) Plugin() api.Plugin {
	return api.Plugin{Name: "block-modules", Setup: func(build api.PluginBuild) {
		// Resolve every import, including entries, exclusively against the snapshot.
		build.OnResolve(api.OnResolveOptions{Filter: "."}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			// Keep archive expansion and source provenance consistent across callbacks.
			release, err := m.mtx.Lock(m.ctx)
			if err != nil {
				return api.OnResolveResult{}, err
			}
			defer release()

			// Leave the host's shared modules as bare imports for its import map.
			id, _, _ := web_pkg.CheckStripWebPkgIdPrefix(args.Path)
			if args.Kind != api.ResolveEntryPoint && slices.Contains(m.external, id) {
				if args.Kind == api.ResolveJSRequireCall {
					return api.OnResolveResult{Path: args.Path, Namespace: "shared"}, nil
				}
				return api.OnResolveResult{Path: args.Path, External: true}, nil
			}

			// Resolve source files and npm imports in the block namespace.
			resolved, err := m.resolve(args.Path, args.Importer, args.Kind)
			return api.OnResolveResult{Path: resolved, Namespace: "blocks"}, err
		})

		// Bridge static CommonJS requires to the same shared ESM singleton, including subpaths.
		build.OnLoad(api.OnLoadOptions{Filter: ".", Namespace: "shared"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			contents := "import * as shared from " + strconv.Quote(args.Path) + "; module.exports = shared;"
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderJS}, nil
		})

		// Read only resolved block paths.
		build.OnLoad(api.OnLoadOptions{Filter: ".", Namespace: "blocks"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			// Read bytes under the same lock used by npm resolution.
			release, err := m.mtx.Lock(m.ctx)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			defer release()
			data, found, err := m.read(args.Path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			if !found {
				return api.OnLoadResult{}, errors.Errorf("module is missing: %s", args.Path)
			}

			// Let esbuild choose the loader from the file extension.
			contents := string(data)
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderDefault}, nil
		})
	}}
}

// InputManifest returns sorted provenance once the build's callbacks have ended.
func (m *Modules) InputManifest() *builder.InputManifest {
	result := &builder.InputManifest{Files: make([]*builder.InputManifest_File, 0, len(m.inputs))}
	for _, file := range m.inputs {
		result.Files = append(result.Files, file)
	}
	result.SortFiles()
	return result
}

// read loads a virtual file, lazily expanding only the npm packages it visits.
// The caller holds mtx while modifying the archive cache and provenance.
func (m *Modules) read(name string) ([]byte, bool, error) {
	// Reject paths outside the source snapshot before touching either input.
	if err := m.ctx.Err(); err != nil {
		return nil, false, err
	}
	name = path.Clean(name)
	rel, valid := strings.CutPrefix(name, "project/")
	if !valid || !fs.ValidPath(rel) {
		return nil, false, errors.New("module path escapes source snapshot")
	}

	// Locate the innermost npm package, including alias and nested installs.
	for dir := path.Dir(name); dir != "project" && dir != "."; dir = path.Dir(dir) {
		pkg, found := m.packages[dir]
		if !found {
			continue
		}
		files, found := m.archives[pkg.Integrity]
		if !found {
			// Read verified persisted bytes without any dependency registry access.
			data, exists, err := m.dependencies.Get(m.ctx, []byte(pkg.Integrity))
			if err != nil {
				return nil, false, err
			}
			if !exists {
				return nil, false, errors.Errorf("verified npm tarball is missing: %s", pkg.Key)
			}
			files, err = readArchive(m.ctx, data, &m.remaining)
			if err != nil {
				return nil, false, errors.Wrap(err, pkg.Key)
			}
			m.archives[pkg.Integrity] = files
		}
		data, exists := files[strings.TrimPrefix(name, dir+"/")]
		return data, exists, nil
	}

	// Read source files directly through the caller's filesystem interface.
	info, err := fs.Stat(m.source, rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.IsDir() {
		return nil, false, nil
	}

	// Capture the bytes and their identity for Bldr's source change detection.
	data, err := fs.ReadFile(m.source, rel)
	if err != nil {
		return nil, false, err
	}
	digest := sha256.Sum256(data)
	m.inputs[rel] = &builder.InputManifest_File{Path: rel, Identity: &builder.InputManifest_FileIdentity{SizeBytes: uint64(len(data)), Sha256: digest[:]}}
	return data, true, nil
}

// resolve selects a source path or a browser npm entry for one import.
func (m *Modules) resolve(specifier, importer string, kind api.ResolveKind) (string, error) {
	// Resolve entrypoints and relative imports within the virtual project root.
	if kind == api.ResolveEntryPoint || strings.HasPrefix(specifier, "/") {
		return m.resolveFile(path.Join("project", specifier))
	}
	if strings.HasPrefix(specifier, ".") {
		return m.resolveFile(path.Join(path.Dir(importer), specifier))
	}

	// Follow Node's nearest node_modules rule using Bun's pinned install keys.
	id, subpath, err := web_pkg.CheckStripWebPkgIdPrefix(specifier)
	if err != nil {
		return "", err
	}
	for dir := path.Dir(importer); dir != "."; dir = path.Dir(dir) {
		root := path.Join(dir, "node_modules", id)
		if _, found := m.packages[root]; found {
			return m.resolvePackage(root, subpath, kind)
		}
	}
	return "", errors.Errorf("npm import is not pinned in bun.lock: %s", specifier)
}

// resolveFile applies browser extension and directory resolution to a block path.
func (m *Modules) resolveFile(name string) (string, error) {
	// Try exact files and extensionless TypeScript and JavaScript imports.
	candidates := []string{name}
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js", ".css", ".json", ".mjs", ".cjs"} {
		candidates = append(candidates, name+ext)
	}

	// Follow esbuild's TypeScript rewrites only after exact and appended files.
	var rewrites []string
	switch path.Ext(name) {
	case ".js", ".jsx":
		rewrites = []string{".ts", ".tsx"}
	case ".mjs":
		rewrites = []string{".mts"}
	case ".cjs":
		rewrites = []string{".cts"}
	}
	for _, ext := range rewrites {
		candidates = append(candidates, strings.TrimSuffix(name, path.Ext(name))+ext)
	}
	for _, candidate := range candidates {
		_, found, err := m.read(candidate)
		if err != nil {
			return "", err
		}
		if found {
			return candidate, nil
		}
	}

	// Resolve directory mains without recursively re-entering the same directory.
	data, found, err := m.read(path.Join(name, "package.json"))
	if err != nil {
		return "", err
	}
	if found {
		var parser fastjson.Parser
		metadata, err := parser.ParseBytes(data)
		if err != nil {
			return "", err
		}
		for _, field := range []string{"browser", "module", "main"} {
			if entry := string(metadata.GetStringBytes(field)); entry != "" {
				entry = path.Clean(entry)
				if !fs.ValidPath(entry) || entry == "." {
					return "", errors.New("npm main path escapes package")
				}
				return m.resolveFile(path.Join(name, entry))
			}
		}
	}
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js", ".json", ".mjs", ".cjs"} {
		candidate := path.Join(name, "index"+ext)
		_, found, err := m.read(candidate)
		if err != nil {
			return "", err
		}
		if found {
			return candidate, nil
		}
	}
	return "", errors.Errorf("module cannot be resolved from blocks: %s", name)
}

// resolvePackage respects export maps before falling back to legacy package mains.
func (m *Modules) resolvePackage(root, subpath string, kind api.ResolveKind) (string, error) {
	// Read the package metadata from its verified archive.
	data, found, err := m.read(path.Join(root, "package.json"))
	if err != nil {
		return "", err
	}
	if !found {
		return m.resolveFile(path.Join(root, subpath))
	}
	var parser fastjson.Parser
	metadata, err := parser.ParseBytes(data)
	if err != nil {
		return "", errors.Wrap(err, "npm package.json")
	}

	// Export maps select only declared paths, with browser and import conditions.
	if exports := metadata.Get("exports"); exports != nil {
		key := "."
		if subpath != "" {
			key = "./" + subpath
		}
		target := packageExport(exports, key, kind)
		if !strings.HasPrefix(target, "./") || !fs.ValidPath(strings.TrimPrefix(target, "./")) {
			return "", errors.Errorf("npm package does not export %s: %s", key, root)
		}
		return m.resolveFile(path.Join(root, target))
	}

	// Legacy packages expose their subpaths and browser/module/main entry fields.
	return m.resolveFile(path.Join(root, subpath))
}

// packageExport selects exact and wildcard subpaths, then ordered conditions.
func packageExport(value *fastjson.Value, key string, kind api.ResolveKind) string {
	// Select the most specific declared package subpath.
	if object := value.GetObject(); object != nil {
		var selected *fastjson.Value
		var substitution string
		best := -1
		bestLength := 0
		hasSubpaths := false
		object.Visit(func(raw []byte, target *fastjson.Value) {
			// Exact subpaths win over matching wildcard declarations.
			pattern := string(raw)
			if !strings.HasPrefix(pattern, ".") {
				return
			}
			hasSubpaths = true
			if pattern == key {
				selected, best = target, len(key)+1
				substitution = ""
				return
			}

			// Match a single wildcard with Node's longest-prefix precedence.
			prefix, suffix, wildcard := strings.Cut(pattern, "*")
			if wildcard && (len(prefix) > best || len(prefix) == best && len(pattern) > bestLength) && strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) && len(key) >= len(prefix)+len(suffix) {
				selected, best = target, len(prefix)
				bestLength = len(pattern)
				substitution = key[len(prefix) : len(key)-len(suffix)]
			}
		})
		if hasSubpaths {
			target, _ := exportCondition(selected, kind)
			return strings.ReplaceAll(target, "*", substitution)
		}
	}
	if key == "." {
		target, _ := exportCondition(value, kind)
		return target
	}
	return ""
}

// exportCondition follows package declaration order for active browser conditions.
func exportCondition(value *fastjson.Value, kind api.ResolveKind) (string, bool) {
	// String exports are file paths, but null exports remain unavailable.
	if value == nil {
		return "", false
	}
	if value.Type() == fastjson.TypeString {
		return string(value.GetStringBytes()), true
	}
	if value.Type() == fastjson.TypeNull {
		return "", true
	}

	// Export arrays allow ordered fallback paths, skipping null or inactive entries.
	if value.Type() == fastjson.TypeArray {
		for _, entry := range value.GetArray() {
			if target, matched := exportCondition(entry, kind); matched && target != "" {
				return target, true
			}
		}
		return "", true
	}

	// Choose import or require and browser/default conditions in declaration order.
	condition := "import"
	if kind == api.ResolveJSRequireCall || kind == api.ResolveJSRequireResolve {
		condition = "require"
	}
	var result string
	matched := false
	if object := value.GetObject(); object != nil {
		object.Visit(func(key []byte, target *fastjson.Value) {
			if !matched && (string(key) == condition || string(key) == "browser" || string(key) == "module" || string(key) == "default") {
				result, matched = exportCondition(target, kind)
			}
		})
	}
	return result, matched
}
