//go:build !js

package bldr_web_bundler_esbuild_build

import (
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

func GoVendorTsResolverPlugin(projectRoot string) esbuild.Plugin {
	return esbuild.Plugin{
		Name: "go-vendor-ts-resolver",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^@go/.*\.js$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
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

				subPath := filepath.FromSlash(strings.TrimPrefix(args.Path, "@go/"))
				jsPath := filepath.Join(projectRoot, "vendor", subPath)

				if fileExists(jsPath) {
					result.Path = jsPath
					return result, nil
				}

				tsPath := strings.TrimSuffix(jsPath, ".js") + ".ts"
				if fileExists(tsPath) {
					result.Path = tsPath
					return result, nil
				}

				tsxPath := strings.TrimSuffix(jsPath, ".js") + ".tsx"
				if fileExists(tsxPath) {
					result.Path = tsxPath
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
