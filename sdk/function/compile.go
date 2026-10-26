//go:build !goscript

package s4wave_function

import (
	"strings"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// Compile compiles the TypeScript source of a PureFunction to the ES module a
// runner evaluates. A PureFunction imports nothing, so an import fails.
func Compile(source string) (string, error) {
	// Build the source as one module, failing on any import.
	res := esbuild_api.Build(esbuild_api.BuildOptions{
		Stdin: &esbuild_api.StdinOptions{
			Contents:   source,
			Sourcefile: "function.ts",
			Loader:     esbuild_api.LoaderTS,
		},
		Bundle:   true,
		Format:   esbuild_api.FormatESModule,
		Target:   esbuild_api.ES2024,
		Platform: esbuild_api.PlatformNeutral,
		Charset:  esbuild_api.CharsetUTF8,
		Plugins: []esbuild_api.Plugin{{
			Name: "pure-function-imports",
			Setup: func(b esbuild_api.PluginBuild) {
				b.OnResolve(esbuild_api.OnResolveOptions{Filter: ".*"}, func(args esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
					return esbuild_api.OnResolveResult{}, errors.Errorf("a PureFunction cannot import %q", args.Path)
				})
			},
		}},
	})

	// Report the first errors with their locations.
	if len(res.Errors) != 0 {
		msgs := esbuild_api.FormatMessages(res.Errors, esbuild_api.FormatMessagesOptions{Kind: esbuild_api.ErrorMessage})
		return "", errors.New(strings.TrimSpace(strings.Join(msgs, "\n")))
	}
	if len(res.OutputFiles) != 1 {
		return "", errors.Errorf("compile produced %d files, want 1", len(res.OutputFiles))
	}
	return string(res.OutputFiles[0].Contents), nil
}
