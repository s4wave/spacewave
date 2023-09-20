package entrypoint_electron_bundle

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_npm "github.com/aperturerobotics/bldr/platform/npm"
	"github.com/aperturerobotics/bldr/util/fsutil"
	"github.com/aperturerobotics/bldr/util/npm"
	bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	util_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	"github.com/aperturerobotics/util/exec"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// ElectronDefine returns the define mapping for Electron.
//
// debug enables debug mode.
func ElectronDefine(debug bool) map[string]string {
	return map[string]string{
		"BLDR_IS_ELECTRON": "true",
		"BLDR_DEBUG":       strconv.FormatBool(debug),
	}
}

// EsbuildLogLevel is the log level when bundling the electron bundle.
var EsbuildLogLevel = esbuild.LogLevelInfo

// ElectronBuildOpts are general options for building for Electron.
func ElectronBuildOpts(bldrDistRoot string, minify, debug bool) esbuild.BuildOptions {
	opts := bundle.BrowserBuildOpts(bldrDistRoot, minify)
	opts.Define = ElectronDefine(debug)
	opts.External = []string{"electron", "electron-nightly"}
	opts.LogLevel = EsbuildLogLevel
	return opts
}

// BuildServiceWorkerBundle builds specifically the service worker files.
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify bool) error {
	return bundle.BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify)
}

// BuildPreloadBundle builds the web renderer bundle files.
func BuildPreloadBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, debug bool) error {
	le.Debug("generating electron preload bundle")
	opts := ElectronBuildOpts(bldrDistRoot, minify, debug)
	opts.Define = ElectronDefine(debug)
	opts.EntryPointsAdvanced = nil
	opts.EntryNames = ""
	opts.EntryPoints = []string{
		"web/electron/main/preload.ts",
	}
	opts.Outfile = filepath.Join(buildDir, "preload.mjs")
	// https://github.com/electron/electron/blob/ac031b/docs/tutorial/esm-limitations.md#esm-preload-scripts-must-have-the-mjs-extension
	opts.Format = esbuild.FormatCommonJS
	opts.Platform = esbuild.PlatformNode
	opts.Write = true
	if !minify {
		opts.Sourcemap = esbuild.SourceMapLinked
	} else {
		opts.Sourcemap = esbuild.SourceMapNone
	}

	res := esbuild.Build(opts)
	return util_esbuild.BuildResultToErr(res)
}

// BuildMainBundle builds the electron Main bundle files.
func BuildMainBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, debug bool) error {
	le.Debug("generating electron main bundle")

	opts := ElectronBuildOpts(bldrDistRoot, minify, debug)
	opts.Define = ElectronDefine(debug)
	opts.EntryPointsAdvanced = nil
	opts.EntryNames = ""
	opts.EntryPoints = []string{
		"web/electron/main/index.ts",
	}
	opts.Outfile = filepath.Join(buildDir, "index.mjs")
	opts.Platform = esbuild.PlatformNode
	opts.Write = true
	if !minify {
		opts.Sourcemap = esbuild.SourceMapLinked
	} else {
		opts.Sourcemap = esbuild.SourceMapNone
	}

	FixEsbuildIssue1921(&opts)

	res := esbuild.Build(opts)
	return util_esbuild.BuildResultToErr(res)
}

// BuildWebPkgsBundle builds the web pkg bundle files.
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, plat bldr_platform.Platform, bldrDistRoot, buildDir string, minify, debug bool) error {
	// build to pkgs/
	outDir := filepath.Join(buildDir, "pkgs")

	// make temporary dir to build web pkgs
	buildPkgsDir := filepath.Join(buildDir, "build-web-pkgs")
	if err := fsutil.CleanCreateDir(buildPkgsDir); err != nil {
		return err
	}

	// copy package.json into it
	if err := fsutil.CopyFile(
		filepath.Join(buildPkgsDir, "package.json"),
		filepath.Join(bldrDistRoot, "dist/deps/package.json"),
		0644,
	); err != nil {
		return err
	}

	// npm install
	npmPlat, err := bldr_platform_npm.PlatformToNpm(plat)
	if err != nil {
		return err
	}

	le.
		WithField("npm-platform", npmPlat.Platform).
		WithField("npm-arch", npmPlat.Arch).
		WithField("npm-pkg", []string{"react", "react-dom"}).
		Debug("downloading dist deps with npm")
	archFlags := npmPlat.ToNpmFlags()
	args := []string{"install"}
	args = append(args, npm.NpmFlags...)
	args = append(args, "--prefix", buildPkgsDir)
	args = append(args, archFlags...)
	cmd := exec.NewCmd("npm", args...)
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	// web pkgs we distribute with bldr
	refs := []*web_pkg_esbuild.WebPkgRef{{
		WebPkgID:   "react",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react"),
		Imports:    []string{"index.js"},
	}, {
		WebPkgID:   "react-dom",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react-dom"),
		Imports:    []string{"client.js", "index.js"},
	}, {
		WebPkgID:   "@aptre/bldr",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgID:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr-react"),
		Imports:    []string{"index.ts"},
	}}
	_, _, err = web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		buildDir,
		refs,
		outDir,
		"./pkgs/",
		false,
	)
	if err != nil {
		return err
	}

	if err := fsutil.CleanDir(buildPkgsDir); err != nil {
		return err
	}

	return nil
}

// BuildRendererBundle builds the web renderer bundle files.
func BuildRendererBundle(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir string, minify, debug bool) error {
	le.Debug("generating web renderer bundle")

	// index.html
	distSrcDir := filepath.Join(bldrDistRoot, "web")
	indexHtmlPath := filepath.Join(distSrcDir, "index.html")
	ihtml, err := os.ReadFile(indexHtmlPath)
	if err != nil {
		return err
	}
	rendererHtmlOut := filepath.Join(buildDir, "index.html")
	err = os.WriteFile(rendererHtmlOut, ihtml, 0644)
	if err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := filepath.Join(buildDir, "entrypoint")
	opts := ElectronBuildOpts(bldrDistRoot, minify, debug)
	opts.Outdir = webEntrypointOut
	opts.EntryPointsAdvanced = nil
	opts.EntryNames = ""
	opts.Define = ElectronDefine(debug)
	opts.EntryPoints = []string{
		"web/entrypoint/entrypoint.tsx",
	}
	opts.External = append(opts.External,
		"react",
		"react-dom",
		"@aptre/bldr",
		"@aptre/bldr-react",
	)
	opts.Write = true
	if !minify {
		opts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(opts)
	return util_esbuild.BuildResultToErr(res)
}

// FixEsbuildIssue1921 fixes dynamic esbuild imports failing under node.js.
//
// https://github.com/evanw/esbuild/issues/1921
func FixEsbuildIssue1921(opts *esbuild.BuildOptions) {
	if opts.Banner == nil {
		opts.Banner = make(map[string]string, 1)
	}
	old := opts.Banner["js"]
	if len(old) != 0 {
		old += "\n"
	}
	// https://github.com/evanw/esbuild/issues/1921#issuecomment-1710527349
	opts.Banner["js"] = old + "const require = (await import('node:module')).createRequire(import.meta.url);const __filename = (await import('node:url')).fileURLToPath(import.meta.url);const __dirname = (await import('node:path')).dirname(__filename);"
}

// BuildElectronBundle builds and outputs the web & service worker files.
//
// minify enables file minification in esbuild
// debug enables debug extensions in Electron
func BuildElectronBundle(ctx context.Context, le *logrus.Entry, bldrDistRoot, buildDir string, minify, debug bool) error {
	err := os.MkdirAll(buildDir, 0755)
	if err != nil {
		return err
	}

	// service worker
	if err := BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify); err != nil {
		return err
	}

	// preload
	if err := BuildPreloadBundle(le, bldrDistRoot, buildDir, minify, debug); err != nil {
		return err
	}

	// main
	if err := BuildMainBundle(le, bldrDistRoot, buildDir, minify, debug); err != nil {
		return err
	}

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("native/linux/amd64")
	if err != nil {
		return err
	}
	if err := BuildWebPkgsBundle(ctx, le, bldrNativePlatform, bldrDistRoot, buildDir, minify, debug); err != nil {
		return err
	}

	// renderer bundle
	if err := BuildRendererBundle(ctx, le, bldrDistRoot, buildDir, minify, debug); err != nil {
		return err
	}

	return nil
}

// BuildAsar builds the app asar using the @electron/asar tool.
//
// asarBinPath should be the path to the asar binary.
// buildDir should be pre-prepared using BuildElectronBundle.
// outPath should be the path to the output .asar file
func BuildAsar(ctx context.Context, le *logrus.Entry, buildDir, outPath string) error {
	cmd := npm.NpmExec("@electron/asar", "pack", buildDir, outPath)
	return exec.StartAndWait(ctx, le, cmd)
}

// DownloadElectronRedist downloads the electron redistributable to the destination dir.
// Uses electron@latest.
func DownloadElectronRedist(ctx context.Context, le *logrus.Entry, plat bldr_platform.Platform, buildDir, destDir string, nightly bool) error {
	npmPlat, err := bldr_platform_npm.PlatformToNpm(plat)
	if err != nil {
		return err
	}

	npmDir := filepath.Join(buildDir, "dl-electron")
	if err := fsutil.CleanCreateDir(npmDir); err != nil {
		return err
	}

	npmPkg := "electron"
	if nightly {
		npmPkg = "electron-nightly"
	}

	le.
		WithField("npm-platform", npmPlat.Platform).
		WithField("npm-arch", npmPlat.Arch).
		WithField("npm-pkg", npmPkg).
		Debug("downloading electron with npm")
	archFlags := npmPlat.ToNpmFlags()
	args := []string{"install"}
	args = append(args, npm.NpmFlags...)
	args = append(args, "--prefix", npmDir)
	args = append(args, archFlags...)
	args = append(args, npmPkg)
	cmd := exec.NewCmd("npm", args...)
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	// move the redistributable out of node_modules
	nodeModulesPath := filepath.Join(npmDir, "node_modules")
	electronDistPath := filepath.Join(nodeModulesPath, npmPkg, "dist")
	if err := fsutil.CopyRecursive(destDir, electronDistPath, nil); err != nil {
		return err
	}

	// delete npm dir
	if err := fsutil.CleanDir(npmDir); err != nil {
		return err
	}

	le.Debug("successfully downloaded electron")
	return nil
}

// GetElectronBinName returns the name of the electron binary.
//
// Returns just "electron" if not known.
func GetElectronBinName(plat bldr_platform.Platform) string {
	np, ok := plat.(*bldr_platform.NativePlatform)
	if !ok {
		return "electron"
	}
	switch np.GetGOOS() {
	case "windows":
		return "electron.exe"
	case "darwin":
		// we have to run the native binary inside the .app
		return "Electron.app/Contents/MacOS/Electron"
	default:
		return "electron"
	}
}
