package web_pkg_esbuild

import (
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
		// Splitting:      true,

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

	// bldr provides itself and react via an importmap
	buildOpts.External = append(buildOpts.External, BldrExternal...)

	return buildOpts
}
