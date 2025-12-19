package entrypoint_electron_bundle

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/aperturerobotics/bldr/util/npm"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// ElectronDefine returns the define mapping for Electron.
//
// devMode enables devMode mode.
func ElectronDefine(devMode bool) map[string]string {
	return map[string]string{
		"BLDR_IS_ELECTRON": "true",
		"BLDR_DEBUG":       strconv.FormatBool(devMode),
	}
}

// EsbuildLogLevel is the log level when bundling the electron entrypoint_browser_bundle.
var EsbuildLogLevel = esbuild.LogLevelWarning

// ElectronBuildOpts are general options for building for Electron.
func ElectronBuildOpts(bldrDistRoot string, minify, devMode bool) esbuild.BuildOptions {
	opts := entrypoint_browser_bundle.BrowserBuildOpts(bldrDistRoot, minify)
	opts.Define = ElectronDefine(devMode)
	opts.External = []string{"electron"}
	opts.LogLevel = EsbuildLogLevel
	return opts
}

// BuildServiceWorkerBundle builds specifically the service worker files.
//
// Returns the path to the service worker .mjs file
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) (string, error) {
	return entrypoint_browser_bundle.BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
}

// BuildPreloadBundle builds the web renderer bundle files.
func BuildPreloadBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) error {
	le.Debug("generating electron preload bundle")
	opts := ElectronBuildOpts(bldrDistRoot, minify, devMode)
	opts.Define = ElectronDefine(devMode)
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
	return bldr_esbuild_build.BuildResultToErr(res)
}

// BuildMainBundle builds the electron Main bundle files.
func BuildMainBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) error {
	le.Debug("generating electron main bundle")

	opts := ElectronBuildOpts(bldrDistRoot, minify, devMode)
	opts.Define = ElectronDefine(devMode)
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
	return bldr_esbuild_build.BuildResultToErr(res)
}

// BuildRendererBundle builds the web renderer bundle files.
//
// runtimeSwPath is the path to the service worker js for the entrypoint to load.
// runtimeShwPath is the path to the service worker js for the entrypoint to load.
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
func BuildRendererBundle(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath string,
	minify,
	devMode bool,
) error {
	le.Debug("generating web renderer bundle")

	// index.html
	if err := entrypoint_browser_bundle.BuildRendererIndex(buildDir, ""); err != nil {
		return err
	}

	// entrypoint
	webEntrypointOut := filepath.Join(buildDir, "entrypoint")
	opts := ElectronBuildOpts(bldrDistRoot, minify, devMode)
	opts.Outdir = webEntrypointOut
	opts.EntryPointsAdvanced = nil
	opts.EntryNames = ""
	opts.Define = ElectronDefine(devMode)
	opts.EntryPoints = []string{
		"web/entrypoint/entrypoint.tsx",
	}
	opts.External = append(opts.External, web_pkg_external.BldrExternal...)
	opts.Write = true

	if runtimeJsPath != "" {
		opts.Define["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}

	if runtimeSwPath != "" {
		opts.Define["BLDR_SW_JS"] = strconv.Quote(runtimeSwPath)
	}

	if runtimeShwPath != "" {
		opts.Define["BLDR_SHW_JS"] = strconv.Quote(runtimeShwPath)
	}

	if webStartupSrcPath != "" {
		opts.Define["BLDR_STARTUP_JS"] = strconv.Quote(webStartupSrcPath)
	}

	if !minify {
		opts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(opts)
	return bldr_esbuild_build.BuildResultToErr(res)
}

// FixEsbuildIssue1921 fixes dynamic esbuild imports failing under node.js.

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
// stateDir is the directory where bun will be downloaded if not found in PATH.
// startupFilename is the path to the react component to load on startup (can be empty).
// minify enables file minification in esbuild
// devMode enables devMode extensions in Electron
// entrypointHash, if set, uses /entrypoint/{entrypointHash}/pkgs/...
func BuildElectronBundle(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot, buildDir, startupFilename string, minify, devMode bool) error {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return err
	}

	// service worker
	swFilename, err := BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return err
	}

	// shared worker
	shwFilename, err := entrypoint_browser_bundle.BuildSharedWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return err
	}

	// preload
	if err := BuildPreloadBundle(le, bldrDistRoot, buildDir, minify, devMode); err != nil {
		return err
	}

	// main
	if err := BuildMainBundle(le, bldrDistRoot, buildDir, minify, devMode); err != nil {
		return err
	}

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("native/linux/amd64")
	if err != nil {
		return err
	}

	// build to the entrypoint dir
	entrypointDir := filepath.Join(buildDir, "entrypoint")
	if err := entrypoint_browser_bundle.BuildWebPkgsBundle(
		ctx,
		le,
		stateDir,
		bldrNativePlatform,
		bldrDistRoot,
		entrypointDir,
		"/entrypoint/", // set the pathPrefix to /entrypoint/ so web pkg paths are correct
		minify,
		devMode,
	); err != nil {
		return err
	}

	// the renderer is at /entrypoint/pkgs/@aptre/bldr/
	runtimePathPrefix := "../../../../"
	runtimeSwPath := runtimePathPrefix + swFilename
	runtimeShwPath := runtimePathPrefix + shwFilename

	var webStartupSrcPath string
	if startupFilename != "" {
		webStartupSrcPath = runtimePathPrefix + startupFilename
	}

	// renderer bundle
	if err := BuildRendererBundle(
		ctx,
		le,
		bldrDistRoot,
		buildDir,
		"",
		runtimeSwPath,
		runtimeShwPath,
		webStartupSrcPath,
		minify,
		devMode,
	); err != nil {
		return err
	}

	return nil
}

// BuildAsar builds the app asar using the @electron/asar tool.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// buildDir should be pre-prepared using BuildElectronBundle.
// outPath should be the path to the output .asar file
func BuildAsar(ctx context.Context, le *logrus.Entry, stateDir, buildDir, outPath string) error {
	cmd, err := npm.BunX(ctx, le, stateDir, "@electron/asar", "pack", buildDir, outPath)
	if err != nil {
		return err
	}
	return exec.StartAndWait(ctx, le, cmd)
}

// DownloadElectronRedist downloads the electron redistributable to the destination dir.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// If npmPkg is empty, defaults to latest.
func DownloadElectronRedist(ctx context.Context, le *logrus.Entry, stateDir string, plat bldr_platform.Platform, buildDir, destDir string, npmPkg string) error {
	npmDir := filepath.Join(buildDir, "dl-electron")
	if err := fsutil.CleanCreateDir(npmDir); err != nil {
		return err
	}

	// Create an empty package.json to prevent bun from traversing up to parent directories
	pkgJsonPath := filepath.Join(npmDir, "package.json")
	if err := os.WriteFile(pkgJsonPath, []byte("{}"), 0o644); err != nil {
		return err
	}

	// use the latest version if not defined
	if npmPkg == "" {
		npmPkg = "electron@latest"
	}

	// trim the version from the name
	npmPkgName := npmPkg
	npmPkgVerIdx := strings.LastIndex(npmPkgName, "@")
	if npmPkgVerIdx > 0 {
		npmPkgName = npmPkgName[:npmPkgVerIdx]
	}

	le.
		WithField("npm-pkg", npmPkg).
		Debug("downloading electron with bun")
	cmd, err := npm.BunAdd(ctx, le, stateDir, "--cwd", npmDir, npmPkg)
	if err != nil {
		return err
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	// move the redistributable out of node_modules
	nodeModulesPath := filepath.Join(npmDir, "node_modules")
	electronDistPath := filepath.Join(nodeModulesPath, npmPkgName, "dist")
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
