package bldr_web_bundler_esbuild_build

import (
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

// ExternalNodeModulesPlugin creates an esbuild plugin that marks modules resolved within
// any 'node_modules' directory as external.
// This is useful to prevent bundling node dependencies when the target environment
// (like Node.js) can resolve them at runtime.
func ExternalNodeModulesPlugin() esbuild.Plugin {
	return esbuild.Plugin{
		Name: "external-node-modules",
		Setup: func(build esbuild.PluginBuild) {
			// Intercept resolution of modules.
			build.OnResolve(esbuild.OnResolveOptions{
				Filter:    `.`,
				Namespace: "file",
			},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					var result esbuild.OnResolveResult
					if args.Importer == "bldr-external-node-modules" {
						return result, nil
					}

					// Let esbuild resolve the path first.
					resolveResult := build.Resolve(args.Path, esbuild.ResolveOptions{
						ResolveDir: args.ResolveDir,
						Kind:       args.Kind,
						Importer:   "bldr-external-node-modules",
						Namespace:  "file",
					})

					// Check if the resolved path points to a file within a node_modules directory.
					// Use platform-independent path separator checks.
					// Check for both absolute and relative paths to node_modules.
					isNodeModule := strings.Contains(resolveResult.Path, "/node_modules/") ||
						strings.Contains(resolveResult.Path, "\\node_modules\\") ||
						strings.HasPrefix(resolveResult.Path, "node_modules/") ||
						strings.HasPrefix(resolveResult.Path, "node_modules\\")
					if isNodeModule {
						// If it is, mark it as external.
						// Return the original path requested, not the resolved path.
						return esbuild.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// Otherwise, let esbuild handle it normally.
					// Return an empty result, indicating not handled by this plugin.
					return esbuild.OnResolveResult{}, nil
				},
			)
		},
	}
}
