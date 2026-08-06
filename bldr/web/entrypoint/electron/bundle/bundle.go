//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/s4wave/spacewave/bldr/util/npm"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
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

const (
	electronStableBootEntrypointPath = "./boot.mjs"
	electronRendererEntrypointPath   = "entrypoint/entrypoint.mjs"
)

// BuildPreloadBundle builds the Electron preload through the direct owner.
func BuildPreloadBundle(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir string,
	minify,
	devMode bool,
) error {
	le.Debug("generating electron preload bundle")
	return buildElectronScript(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		buildDir,
		"preload",
		"web/electron/main/preload.ts",
		"preload.mjs",
		"cjs",
		minify,
		devMode,
		false,
	)
}

// BuildMainBundle builds the Electron main process through the direct owner.
func BuildMainBundle(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir string,
	minify,
	devMode bool,
) error {
	le.Debug("generating electron main bundle")
	return buildElectronScript(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		buildDir,
		"main",
		"web/electron/main/index.ts",
		"index.mjs",
		"es",
		minify,
		devMode,
		true,
	)
}

// BuildRendererBundle builds the web renderer bundle files.
//
// runtimeSwPath is the path to the service worker js for the entrypoint to load.
// runtimeShwPath is the path to the service worker js for the entrypoint to load.
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
func BuildRendererBundle(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath string,
	minify,
	devMode bool,
) error {
	le.Debug("generating Electron renderer bundle")
	defines := ElectronDefine(devMode)
	if runtimeJsPath != "" {
		defines["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}
	if runtimeSwPath != "" {
		defines["BLDR_SW_JS"] = strconv.Quote(runtimeSwPath)
	}
	if runtimeShwPath != "" {
		defines["BLDR_SHW_JS"] = strconv.Quote(runtimeShwPath)
	}
	if webStartupSrcPath != "" {
		defines["BLDR_STARTUP_JS"] = strconv.Quote(webStartupSrcPath)
	}
	result, err := entrypoint_browser_bundle.BuildRenderer(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		buildDir,
		entrypoint_browser_bundle.ConfigFreeRendererOpts{
			OutputDir:     filepath.Join(buildDir, "entrypoint"),
			PublicPath:    "/entrypoint/",
			Defines:       defines,
			ExtraExternal: []string{"electron"},
			Minify:        minify,
			Sourcemaps:    !minify,
		},
	)
	if err != nil {
		return err
	}
	if result.JSPath != electronRendererEntrypointPath {
		return errors.Errorf("Electron renderer output is %q", result.JSPath)
	}
	return nil
}

func BuildElectronRendererIndex(buildDir string, importMap web_entrypoint_index.ImportMap) error {
	return entrypoint_browser_bundle.BuildRendererIndex(
		buildDir,
		electronStableBootEntrypointPath,
		importMap,
	)
}

func WriteElectronStableBootFiles(buildDir, serviceWorkerFilename, sharedWorkerFilename string) error {
	if err := entrypoint_browser_bundle.WriteStableBootAsset(buildDir); err != nil {
		return err
	}
	entrypointInfo, err := os.Stat(filepath.Join(buildDir, electronRendererEntrypointPath))
	if err != nil {
		return errors.Wrap(err, "stat electron entrypoint bundle")
	}
	return entrypoint_browser_bundle.WriteBuildManifest(
		buildDir,
		&entrypoint_browser_bundle.BuildManifest{
			Entrypoint:                 electronRendererEntrypointPath,
			EntrypointDecompressedSize: entrypointInfo.Size(),
			ServiceWorker:              serviceWorkerFilename,
			SharedWorker:               sharedWorkerFilename,
			Wasm:                       electronRendererEntrypointPath,
			AutoStart:                  true,
		},
	)
}

// BuildElectronBundle builds and outputs the web & service worker files.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// startupFilename is the path to the react component to load on startup (can be empty).
// minify enables JavaScript minification.
// devMode enables devMode extensions in Electron
// entrypointHash, if set, uses /entrypoint/{entrypointHash}/pkgs/...
func BuildElectronBundle(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot, buildDir, startupFilename string, minify, devMode bool) error {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return err
	}
	if _, err := entrypoint_browser_bundle.EnsureBldrDistDepsInstall(ctx, le, stateDir, bldrDistRoot); err != nil {
		return err
	}

	// service worker
	swFilename, err := entrypoint_browser_bundle.BuildServiceWorkerBundle(
		ctx, le, stateDir, bldrDistRoot, buildDir, minify, !minify, devMode,
	)
	if err != nil {
		return err
	}

	// shared worker
	shwFilename, err := entrypoint_browser_bundle.BuildSharedWorkerBundle(
		ctx, le, stateDir, bldrDistRoot, buildDir, minify, !minify, devMode,
	)
	if err != nil {
		return err
	}

	if err := buildElectronScript(
		ctx, le, stateDir, bldrDistRoot, buildDir,
		"preload", "web/electron/main/preload.ts", "preload.mjs", "cjs",
		minify, devMode, false,
	); err != nil {
		return err
	}
	if err := buildElectronScript(
		ctx, le, stateDir, bldrDistRoot, buildDir,
		"main", "web/electron/main/index.ts", "index.mjs", "es",
		minify, devMode, true,
	); err != nil {
		return err
	}

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("desktop/linux/amd64")
	if err != nil {
		return err
	}

	// build to the entrypoint dir
	entrypointDir := filepath.Join(buildDir, "entrypoint")
	webPkgImportMap, err := entrypoint_browser_bundle.BuildWebPkgsBundle(
		ctx,
		le,
		stateDir,
		bldrNativePlatform,
		bldrDistRoot,
		entrypointDir,
		// Match browser web-package URLs so import-map and sibling imports share
		// module identity.
		"/entrypoint",
		minify,
		!minify,
		devMode,
	)
	if err != nil {
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
		stateDir,
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

	if err := WriteElectronStableBootFiles(buildDir, swFilename, shwFilename); err != nil {
		return err
	}

	// Render index.html with the import map from the web pkg build.
	if err := BuildElectronRendererIndex(buildDir, webPkgImportMap); err != nil {
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
//
// When plat is a NativePlatform whose goos/goarch differ from the host,
// npm_config_* and ELECTRON_INSTALL_* are set so Electron's installer fetches
// the target redistributable instead of the host's. Without this,
// cross-platform release builds land with a host-arch electron in dist/, which
// then fails downstream branding / packaging steps that expect target-arch
// layout.
func DownloadElectronRedist(ctx context.Context, le *logrus.Entry, stateDir string, plat bldr_platform.Platform, buildDir, destDir string, npmPkg string) error {
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

	// Build the cross-download env. Electron 42+ reads ELECTRON_INSTALL_*
	// in its install-electron binary; older postinstall paths read
	// npm_config_* through @electron/get.
	var extraEnv []string
	if np, ok := plat.(*bldr_platform.NativePlatform); ok {
		nodePlat := npm.GOOSToNodePlatform(np.GetGOOS())
		nodeArch := npm.GOARCHToNodeArch(np.GetGOARCH())
		if nodePlat != "" {
			extraEnv = append(extraEnv, "npm_config_platform="+nodePlat)
			extraEnv = append(extraEnv, "ELECTRON_INSTALL_PLATFORM="+nodePlat)
		}
		if nodeArch != "" {
			extraEnv = append(extraEnv, "npm_config_arch="+nodeArch)
			extraEnv = append(extraEnv, "ELECTRON_INSTALL_ARCH="+nodeArch)
		}
	}

	// install electron (cached: skips if package string + env unchanged)
	npmDir := filepath.Join(buildDir, "dl-electron")
	le.
		WithField("npm-pkg", npmPkg).
		WithField("extra-env", extraEnv).
		Debug("downloading electron with bun")
	if err := npm.EnsureBunAdd(ctx, le, stateDir, npmDir, npmPkg, extraEnv...); err != nil {
		return err
	}

	// copy the redistributable out of node_modules
	nodeModulesPath := filepath.Join(npmDir, "node_modules")
	electronDistPath := filepath.Join(nodeModulesPath, npmPkgName, "dist")
	if _, err := os.Stat(electronDistPath); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "stat electron dist")
		}
		cmdPath := filepath.Join(nodeModulesPath, ".bin", "install-electron")
		if runtime.GOOS == "windows" {
			cmdPath += ".cmd"
		}
		cmd := exec.NewCmd(ctx, cmdPath)
		if _, statErr := os.Stat(cmdPath); statErr != nil {
			if !os.IsNotExist(statErr) {
				return errors.Wrap(statErr, "stat install electron binary")
			}
			installPath := filepath.Join(nodeModulesPath, npmPkgName, "install.js")
			if _, installErr := os.Stat(installPath); installErr != nil {
				if os.IsNotExist(installErr) {
					return errors.Errorf("install electron binary missing: %s", cmdPath)
				}
				return errors.Wrap(installErr, "stat electron install script")
			}
			cmd = exec.NewCmd(ctx, "node", installPath)
		}
		cmd.Dir = npmDir
		cmd.Env = append(cmd.Env, extraEnv...)
		if err := exec.StartAndWait(ctx, le, cmd); err != nil {
			return errors.Wrap(err, "install electron binary")
		}
	}
	if err := fsutil.CopyRecursive(destDir, electronDistPath, nil); err != nil {
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
