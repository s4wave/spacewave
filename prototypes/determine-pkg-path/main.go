package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/evanw/esbuild/pkg/api"
)

func newResolvePlugin(pkgName string) api.Plugin {
	return api.Plugin{
		Name: pkgName + "-resolver",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{
				Filter:    `^` + pkgName + `$`,
				Namespace: "file",
			},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					result := build.Resolve(pkgName, api.ResolveOptions{
						Kind:       api.ResolveJSImportStatement,
						ResolveDir: args.ResolveDir,
						Namespace:  "resolve-pkg",
					})
					if len(result.Errors) > 0 {
						return api.OnResolveResult{Errors: result.Errors}, nil
					}
					fmt.Println(pkgName+" is resolved to:", result.Path)
					return api.OnResolveResult{Path: result.Path, External: true}, nil
				})
		},
	}
}

func main() {
	pkgName := "react"
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{"app.js"},
		Bundle:      true,
		Plugins:     []api.Plugin{newResolvePlugin(pkgName)},
		// Outfile:     "out.js",
		// Write:       true,
	})

	err := os.WriteFile("app.js", []byte("import("+strconv.Quote(pkgName)+")"), 0644)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			fmt.Println(err.Text)
		}
		os.Exit(1)
	}
}
