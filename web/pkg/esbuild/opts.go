package web_pkg_esbuild

import (
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// BuildEsbuildBuildOpts constructs the base esbuild build opts.
//
// publicPath is the base public path, e.x.: /p/{plugin-id} or /b/pkg/{pkg-id}
func BuildEsbuildBuildOpts(
	le *logrus.Entry,
	codeRootPath,
	outPath,
	publicPath string,
	isRelease,
	useHashes bool,
) *esbuild_api.BuildOptions {
	entryNames := "[dir]/[name]"
	if useHashes {
		entryNames += "-[hash]"
	}

	assetNames := "assets/[ext]/[name]"
	if useHashes {
		assetNames += "-[hash]"
	}

	buildOpts := &esbuild_api.BuildOptions{
		AbsWorkingDir: codeRootPath,
		Outdir:        outPath,
		PublicPath:    publicPath,

		EntryNames: entryNames,
		AssetNames: assetNames,

		LogLevel:    esbuild_api.LogLevelWarning,
		Platform:    esbuild_api.PlatformBrowser,
		Format:      esbuild_api.FormatESModule,
		Target:      esbuild_api.ES2022,
		TreeShaking: esbuild_api.TreeShakingTrue,

		AllowOverwrite: true,
		Bundle:         true,
		Metafile:       true,
		Write:          true,

		// https://github.com/evanw/esbuild/issues/399
		Splitting: true,

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
		//	buildOpts.Sourcemap = esbuild_api.SourceMapInline
		buildOpts.Sourcemap = esbuild_api.SourceMapLinked
	}

	// add common loader types
	addLoader := func(ext string, typ esbuild_api.Loader) {
		if _, ok := buildOpts.Loader[ext]; !ok {
			buildOpts.Loader[ext] = typ
		}
	}

	// use file loader for these types
	useFileLoader := []string{
		"woff",
		"woff2",
		"png",
		"jpg",
		"jpeg",
		"svg",
		"gif",
		"tif",
		"tiff",
	}
	for _, ext := range useFileLoader {
		addLoader("."+ext, esbuild_api.LoaderFile)
	}

	// add css module loader
	// https://esbuild.github.io/content-types/#local-css
	addLoader(".module.css", esbuild_api.LoaderLocalCSS)

	// add text loaders for common text file types
	textLoaderExts := []string{
		".txt",
		".md",
		".csv",
		".tsv",
		".yaml",
		".yml",
		".json",
		".toml",
		".ini",
		".env",
		".rst",
		".log",
		".conf",
		".cfg",
	}
	for _, ext := range textLoaderExts {
		addLoader(ext, esbuild_api.LoaderText)
	}

	// bldr provides itself and react via an importmap
	buildOpts.External = append(buildOpts.External, web_pkg_external.BldrExternal...)

	return buildOpts
}

// MergeEsbuildBuildOpts merges esbuild build options.
func MergeEsbuildBuildOpts(target, source *esbuild_api.BuildOptions) {
	mergeValueIfSet(&target.Target, source.Target)
	if len(source.Engines) != 0 {
		target.Engines = source.Engines
	}
	mergeValueIfSet(&target.LogLevel, source.LogLevel)
	mergeValueIfSet(&target.LogLimit, source.LogLimit)
	mergeMapOverwrite(&target.LogOverride, source.LogOverride)
	mergeMapOverwrite(&target.Supported, source.Supported)
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
	mergeMapOverwrite(&target.Define, source.Define)
	if len(source.Pure) != 0 {
		target.Pure = append(target.Pure, source.Pure...)
	}
	mergeValueIfSet(&target.KeepNames, source.KeepNames)
	mergeValueIfSet(&target.Platform, source.Platform)
	if len(source.External) != 0 {
		target.External = append(target.External, source.External...)
	}
	mergeValueIfSet(&target.Packages, source.Packages)
	mergeMapOverwrite(&target.Alias, source.Alias)
	if len(source.MainFields) != 0 {
		target.MainFields = append(target.MainFields, source.MainFields...)
	}
	if len(source.Conditions) != 0 {
		target.Conditions = append(target.Conditions, source.Conditions...)
	}
	mergeMapOverwrite(&target.Loader, source.Loader)
	if len(source.ResolveExtensions) != 0 {
		target.ResolveExtensions = append(target.ResolveExtensions, source.ResolveExtensions...)
	}
	mergeValueIfSet(&target.Tsconfig, source.Tsconfig)
	mergeMapOverwrite(&target.OutExtension, source.OutExtension)
	if len(source.Inject) != 0 {
		target.Inject = append(target.Inject, source.Inject...)
	}
	mergeMapOverwrite(&target.Banner, source.Banner)
	mergeMapOverwrite(&target.Footer, source.Footer)
	mergeValueIfSet(&target.EntryNames, source.EntryNames)
	mergeValueIfSet(&target.ChunkNames, source.ChunkNames)
	mergeValueIfSet(&target.AssetNames, source.AssetNames)
	if len(source.Plugins) != 0 {
		target.Plugins = append(target.Plugins, source.Plugins...)
	}
}
