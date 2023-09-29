package main

import (
	"errors"

	esbuild_api "github.com/evanw/esbuild/pkg/api"
)

func main() {
	res := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"entry.js"},

		LogLevel: esbuild_api.LogLevelVerbose,
		Platform: esbuild_api.PlatformBrowser,
		Format:   esbuild_api.FormatESModule,

		Bundle:  true,
		Write:   true,
		Outfile: "out.js",

		Plugins: []esbuild_api.Plugin{buildPlugin()},
		Loader: map[string]esbuild_api.Loader{
			".json": esbuild_api.LoaderFile,
		},
	})
	if len(res.Errors) != 0 {
		panic(res.Errors[0].Text)
	}
}

func buildPlugin() esbuild_api.Plugin {
	return esbuild_api.Plugin{
		Name: "logger",
		Setup: func(pb esbuild_api.PluginBuild) {
			pb.OnResolve(esbuild_api.OnResolveOptions{
				Filter:    ".",
				Namespace: "file",
			}, func(ora esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				var result esbuild_api.OnResolveResult
				if ora.Importer == "logger" {
					return result, nil
				}

				resResult := pb.Resolve("@mantine/core/package.json", esbuild_api.ResolveOptions{
					// Importer: ora.Importer,
					// Namespace:  ora.Namespace,
					// Namespace:  "logger",
					Namespace:  "file",
					Importer:   "logger",
					ResolveDir: ora.ResolveDir,
					Kind:       esbuild_api.ResolveJSImportStatement,
				})
				if len(resResult.Errors) != 0 {
					return result, errors.New(resResult.Errors[0].Text)
				}

				return result, nil
			})
		},
	}
}
