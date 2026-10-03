//go:build !js

package bldr_web_bundler_esbuild_build

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
)

const localModulePrefix = "github.com/s4wave/spacewave/"

var bldrDistSourcePrefixes = []string{
	"devtool/",
	"manifest/",
	"plugin/",
	"resource/",
	"sdk/",
	"web/",
}

type goVendorTsResolver struct {
	sourcePath     string
	distSourcePath string
	localModule    string
}

func newGoVendorTsResolver(sourcePath, distSourcePath string) goVendorTsResolver {
	if distSourcePath == "" {
		distSourcePath = sourcePath
	}
	return goVendorTsResolver{
		sourcePath:     sourcePath,
		distSourcePath: distSourcePath,
		localModule:    readLocalModulePath(sourcePath),
	}
}

func (r goVendorTsResolver) resolveGoImportPath(importPath string) string {
	if r.localModule != "" {
		if after, ok := strings.CutPrefix(importPath, r.localModule+"/"); ok {
			return filepath.Join(r.sourcePath, filepath.FromSlash(after))
		}
	}

	if after, ok := strings.CutPrefix(importPath, localModulePrefix); ok && r.localModule == "" {
		return filepath.Join(r.sourcePath, filepath.FromSlash(after))
	}

	return filepath.Join(r.distSourcePath, "vendor", filepath.FromSlash(importPath))
}

func (r goVendorTsResolver) resolveDistSourcePath(importPath string) (string, bool) {
	for _, prefix := range bldrDistSourcePrefixes {
		if strings.HasPrefix(importPath, prefix) {
			return filepath.Join(r.distSourcePath, filepath.FromSlash(importPath)), true
		}
	}
	return "", false
}

func readLocalModulePath(projectRoot string) string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			fields := strings.Fields(after)
			if len(fields) != 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func resolveExistingSourcePath(jsPath string) (string, bool) {
	// Prefer the JavaScript source when it exists.
	if fileExists(jsPath) {
		return jsPath, true
	}

	// Resolve a TypeScript source for the JavaScript import.
	tsPath := strings.TrimSuffix(jsPath, ".js") + ".ts"
	if fileExists(tsPath) {
		return tsPath, true
	}

	// Resolve a TSX source when the other source extensions are absent.
	tsxPath := strings.TrimSuffix(jsPath, ".js") + ".tsx"
	if fileExists(tsxPath) {
		return tsxPath, true
	}

	return "", false
}

func (r goVendorTsResolver) resolveEscapedRelativeImport(importer, importPath string) (string, bool) {
	// Require a filesystem importer for relative source resolution.
	if importer == "" || strings.HasPrefix(importer, "\x00") {
		return "", false
	}

	// Require the importer to belong to the flattened dist source tree.
	relImporter, err := filepath.Rel(r.distSourcePath, importer)
	if err != nil || filepath.IsAbs(relImporter) || relImporter == ".." ||
		strings.HasPrefix(relImporter, ".."+string(filepath.Separator)) {
		return "", false
	}

	// Select relative imports that escape the flattened dist source tree.
	target := filepath.Clean(filepath.Join(filepath.Dir(importer), filepath.FromSlash(importPath)))
	relTarget, err := filepath.Rel(r.distSourcePath, target)
	if err != nil || filepath.IsAbs(relTarget) ||
		(relTarget != ".." && !strings.HasPrefix(relTarget, ".."+string(filepath.Separator))) {
		return "", false
	}

	// Resolve the escaped import through its original Go module path.
	modulePath := path.Join(localModulePrefix+"bldr", filepath.ToSlash(relImporter))
	modulePath = path.Clean(path.Join(path.Dir(modulePath), filepath.ToSlash(importPath)))
	return resolveExistingSourcePath(r.resolveGoImportPath(modulePath))
}

func GoVendorTsResolverPlugin(sourcePath, distSourcePath string) esbuild.Plugin {
	resolver := newGoVendorTsResolver(sourcePath, distSourcePath)
	return esbuild.Plugin{
		Name: "go-vendor-ts-resolver",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^@go/.*\.js$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				// Limit Go import resolution to JavaScript imports outside the resolver itself.
				var result esbuild.OnResolveResult
				if args.Importer == "bldr-go-vendor-ts-resolver" {
					return result, nil
				}
				if !strings.HasPrefix(args.Path, "@go/") {
					return result, nil
				}
				if !strings.HasSuffix(args.Path, ".js") {
					return result, nil
				}

				// Locate the Go import in the local source or dist vendor tree.
				subPath := strings.TrimPrefix(args.Path, "@go/")
				jsPath := resolver.resolveGoImportPath(subPath)

				// Resolve the Go import to an existing JavaScript or TypeScript source.
				if sourcePath, ok := resolveExistingSourcePath(jsPath); ok {
					result.Path = sourcePath
					return result, nil
				}

				return result, nil
			})
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^(devtool|manifest|plugin|resource|sdk|web)/.*\.js$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				// Limit dist source resolution to JavaScript imports outside the resolver itself.
				var result esbuild.OnResolveResult
				if args.Importer == "bldr-go-vendor-ts-resolver" {
					return result, nil
				}
				if !strings.HasSuffix(args.Path, ".js") {
					return result, nil
				}

				// Resolve the dist import to an existing source under a supported prefix.
				jsPath, ok := resolver.resolveDistSourcePath(args.Path)
				if !ok {
					return result, nil
				}
				if sourcePath, ok := resolveExistingSourcePath(jsPath); ok {
					result.Path = sourcePath
					return result, nil
				}

				return result, nil
			})
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^\.\.?/.*\.js$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				// Limit escaped source resolution to relative JavaScript imports.
				var result esbuild.OnResolveResult
				if args.Importer == "bldr-go-vendor-ts-resolver" {
					return result, nil
				}
				if !strings.HasPrefix(args.Path, "./") && !strings.HasPrefix(args.Path, "../") {
					return result, nil
				}
				if !strings.HasSuffix(args.Path, ".js") {
					return result, nil
				}

				// Resolve the relative import through the dist vendor tree when it escapes.
				if sourcePath, ok := resolver.resolveEscapedRelativeImport(args.Importer, args.Path); ok {
					result.Path = sourcePath
					return result, nil
				}
				return result, nil
			})
		},
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
