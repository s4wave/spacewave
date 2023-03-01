package plugin_compiler

import esbuild_api "github.com/evanw/esbuild/pkg/api"

// mergeMapOverwrite merges two maps together overwriting values in target.
func mergeMapOverwrite[K comparable, T any](target, source map[K]T) {
	for k, v := range source {
		target[k] = v
	}
}

// mergeValueIfSet overwrites the target value if the source value is not zero.
func mergeValueIfSet[T comparable](target *T, source T) {
	var zero T
	if target != nil && source != zero {
		*target = source
	}
}

// buildEsbuildBuildOpts constructs the base esbuild build opts.
func buildEsbuildBuildOpts(codeRootPath, outAssetsPath, pluginPath string, isRelease bool) *esbuild_api.BuildOptions {
	buildOpts := &esbuild_api.BuildOptions{
		AbsWorkingDir: codeRootPath,
		Outdir:        outAssetsPath,
		PublicPath:    pluginPath,
		EntryNames:    "[dir]/[name]-[hash]",

		LogLevel:    esbuild_api.LogLevelDebug,
		Platform:    esbuild_api.PlatformBrowser,
		Format:      esbuild_api.FormatESModule,
		Target:      esbuild_api.ES2021,
		TreeShaking: esbuild_api.TreeShakingTrue,

		AllowOverwrite: true,
		Bundle:         true,
		Metafile:       true,
		Write:          true,
		Splitting:      true,

		Define:       make(map[string]string),
		Alias:        make(map[string]string),
		Loader:       make(map[string]esbuild_api.Loader),
		OutExtension: make(map[string]string),
		Banner:       make(map[string]string),
		Footer:       make(map[string]string),

		MinifyWhitespace:  isRelease,
		MinifyIdentifiers: isRelease,
		MinifySyntax:      isRelease,
	}
	if !isRelease {
		buildOpts.Sourcemap = esbuild_api.SourceMapInline
	}

	// add common loader types
	addLoader := func(ext string, typ esbuild_api.Loader) {
		if _, ok := buildOpts.Loader[ext]; !ok {
			buildOpts.Loader[ext] = typ
		}
	}
	useFileLoader := []string{"woff", "woff2", "png", "jpg", "jpeg", "svg", "gif", "tif", "tiff"}
	for _, ext := range useFileLoader {
		addLoader("."+ext, esbuild_api.LoaderFile)
	}
	return buildOpts
}

// mergeEsbuildBuildOpts merges esbuild build options.
func mergeEsbuildBuildOpts(target, source *esbuild_api.BuildOptions) {
	mergeValueIfSet(&target.Target, source.Target)
	if len(source.Engines) != 0 {
		target.Engines = source.Engines
	}
	mergeMapOverwrite(target.Supported, source.Supported)
	mergeValueIfSet(&target.MangleProps, source.MangleProps)
	mergeValueIfSet(&target.ReserveProps, source.ReserveProps)
	mergeValueIfSet(&target.MangleQuoted, source.MangleQuoted)
	mergeValueIfSet(&target.Drop, source.Drop)
	mergeValueIfSet(&target.TreeShaking, source.TreeShaking)
	mergeValueIfSet(&target.IgnoreAnnotations, source.IgnoreAnnotations)
	mergeValueIfSet(&target.LegalComments, source.LegalComments)
	mergeValueIfSet(&target.JSX, source.JSX)
	mergeValueIfSet(&target.JSXFactory, source.JSXFactory)
	mergeValueIfSet(&target.JSXImportSource, source.JSXImportSource)
	mergeValueIfSet(&target.JSXDev, source.JSXDev)
	mergeValueIfSet(&target.JSXSideEffects, source.JSXSideEffects)
	mergeMapOverwrite(target.Define, source.Define)
	if len(source.Pure) != 0 {
		target.Pure = append(target.Pure, source.Pure...)
	}
	mergeValueIfSet(&target.KeepNames, source.KeepNames)
	mergeValueIfSet(&target.Platform, source.Platform)
	if len(source.External) != 0 {
		target.External = append(target.External, source.External...)
	}
	mergeValueIfSet(&target.Packages, source.Packages)
	mergeMapOverwrite(target.Alias, source.Alias)
	if len(source.MainFields) != 0 {
		target.MainFields = append(target.MainFields, source.MainFields...)
	}
	if len(source.Conditions) != 0 {
		target.Conditions = append(target.Conditions, source.Conditions...)
	}
	mergeMapOverwrite(target.Loader, source.Loader)
	if len(source.ResolveExtensions) != 0 {
		target.ResolveExtensions = append(target.ResolveExtensions, source.ResolveExtensions...)
	}
	mergeValueIfSet(&target.Tsconfig, source.Tsconfig)
	mergeMapOverwrite(target.OutExtension, source.OutExtension)
	if len(source.Inject) != 0 {
		target.Inject = append(target.Inject, source.Inject...)
	}
	mergeMapOverwrite(target.Banner, source.Banner)
	mergeMapOverwrite(target.Footer, source.Footer)
	mergeValueIfSet(&target.EntryNames, source.EntryNames)
	mergeValueIfSet(&target.ChunkNames, source.ChunkNames)
	mergeValueIfSet(&target.AssetNames, source.AssetNames)
	if len(source.Plugins) != 0 {
		target.Plugins = append(target.Plugins, source.Plugins...)
	}
}
