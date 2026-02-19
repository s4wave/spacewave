//go:build !js

package entrypoint_saucer_bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/bldr/util/npm"
	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// SaucerDefine returns the define mapping for Saucer builds.
func SaucerDefine(devMode bool) map[string]string {
	return map[string]string{
		"BLDR_SAUCER": "true",
		"BLDR_DEBUG":  strconv.FormatBool(devMode),
	}
}

// EsbuildLogLevel is the log level when bundling the saucer entrypoint.
var EsbuildLogLevel = esbuild.LogLevelWarning

// SaucerBuildOpts are general options for building for Saucer.
func SaucerBuildOpts(bldrDistRoot string, minify, devMode bool) esbuild.BuildOptions {
	opts := entrypoint_browser_bundle.BrowserBuildOpts(bldrDistRoot, minify)
	opts.Define = SaucerDefine(devMode)
	opts.LogLevel = EsbuildLogLevel
	return opts
}

// SaucerJSBundle contains the bundled JS files for Saucer.
type SaucerJSBundle struct {
	// BootstrapHTML is the HTML content to load initially.
	BootstrapHTML string
	// EntrypointJS is the entrypoint JavaScript module.
	// Must be served at /entrypoint.mjs.
	EntrypointJS string
}

// BuildSaucerJSBundle builds the JS runtime bundle that runs inside Saucer.
//
// The pkgs (react, @aptre/bldr, etc.) are served via the fetch protocol at /b/pkg/.
// Returns the bootstrap HTML and entrypoint JS.
func BuildSaucerJSBundle(
	le *logrus.Entry,
	bldrDistRoot,
	buildDir string,
	minify bool,
) (*SaucerJSBundle, error) {
	le.Debug("generating saucer JS runtime bundle")

	devMode := !minify

	// Create build directory
	saucerBuildDir := filepath.Join(buildDir, "saucer-js")
	if err := fsutil.CleanCreateDir(saucerBuildDir); err != nil {
		return nil, err
	}

	// Build the entrypoint bundle with external packages
	// These external packages are served via fetch protocol at /b/pkg/
	entrypointOpts := SaucerBuildOpts(bldrDistRoot, minify, devMode)
	entrypointOpts.EntryPointsAdvanced = nil
	entrypointOpts.EntryNames = ""
	entrypointOpts.EntryPoints = []string{
		"web/entrypoint/entrypoint.tsx",
	}
	entrypointOpts.Outfile = filepath.Join(saucerBuildDir, "entrypoint.mjs")
	entrypointOpts.Platform = esbuild.PlatformBrowser
	entrypointOpts.Format = esbuild.FormatESModule
	entrypointOpts.Write = true
	entrypointOpts.Bundle = true
	// Use external packages - they will be loaded via import map from /b/pkg/
	entrypointOpts.External = slices.Clone(web_pkg_external.BldrExternal)

	if !minify {
		entrypointOpts.Sourcemap = esbuild.SourceMapInline
	} else {
		entrypointOpts.Sourcemap = esbuild.SourceMapNone
	}

	entrypointRes := esbuild.Build(entrypointOpts)
	if err := bldr_esbuild_build.BuildResultToErr(entrypointRes); err != nil {
		return nil, fmt.Errorf("building entrypoint: %w", err)
	}

	// Read the built JS file
	entrypointJS, err := os.ReadFile(filepath.Join(saucerBuildDir, "entrypoint.mjs"))
	if err != nil {
		return nil, err
	}

	// Generate bootstrap HTML with import map pointing to /b/pkg/
	// The pkgs are served via the fetch protocol by Go runtime
	importMap := web_pkg_external.GetBldrDistImportMap(bldr_plugin.PluginWebPkgHttpPrefix)
	bootstrapHtml := generateBootstrapHtml(importMap)

	return &SaucerJSBundle{
		BootstrapHTML: bootstrapHtml,
		EntrypointJS:  string(entrypointJS),
	}, nil
}

// generateBootstrapHtml generates the HTML with import map and entrypoint script.
func generateBootstrapHtml(importMap web_entrypoint_index.ImportMap) string {
	return `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    html, body { height: 100%; }
    #bldr-root { display: flex; position: fixed; inset: 0; }
  </style>
</head>
<body>
  <div id="bldr-root"></div>
  <script type="importmap">
` + importMap.String() + `
  </script>
  <script type="module" src="/entrypoint.mjs"></script>
</body>
</html>`
}

// BuildSaucerFromSource builds the saucer binary from source using cmake.
// vendorDir is the Go vendor directory containing bldr-saucer, saucer, and cpp-yamux.
// buildDir is a writable directory for cmake build artifacts.
func BuildSaucerFromSource(
	ctx context.Context,
	le *logrus.Entry,
	vendorDir,
	buildDir,
	outDir string,
	platform bldr_platform.Platform,
) error {
	binName := GetSaucerBinName(platform)
	sourceDir := filepath.Join(vendorDir, "github.com/aperturerobotics/bldr-saucer")
	saucerDir := filepath.Join(vendorDir, "github.com/aperturerobotics/saucer")
	yamuxDir := filepath.Join(vendorDir, "github.com/aperturerobotics/cpp-yamux")

	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}

	// cmake configure (out-of-source build)
	le.Info("configuring saucer build")
	configure := exec.NewCmd(ctx, "cmake",
		"-G", "Ninja",
		"-S", sourceDir,
		"-B", buildDir,
		"-DSAUCER_SOURCE_DIR="+saucerDir,
		"-DYAMUX_SOURCE_DIR="+yamuxDir,
	)
	if err := exec.StartAndWait(ctx, le, configure); err != nil {
		return fmt.Errorf("cmake configure failed: %w", err)
	}

	// cmake build
	le.Info("building saucer from source")
	build := exec.NewCmd(ctx, "cmake", "--build", buildDir)
	if err := exec.StartAndWait(ctx, le, build); err != nil {
		return fmt.Errorf("cmake build failed: %w", err)
	}

	// Copy binary to output directory.
	srcBin := filepath.Join(buildDir, binName)
	destBin := filepath.Join(outDir, binName)
	return fsutil.CopyFile(destBin, srcBin, 0o755)
}

// ResolveSaucerBinary resolves the saucer binary from the @aptre/bldr-saucer npm package.
//
// Checks the cache directory first, falls back to downloading via bun add.
// stateDir is the directory where bun will be downloaded if not found in PATH.
// npmPkg is the npm package specifier (e.g. "@aptre/bldr-saucer@1.0.0").
func ResolveSaucerBinary(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	outDir,
	cacheDir string,
	platform bldr_platform.Platform,
	npmPkg string,
) error {
	binName := GetSaucerBinName(platform)
	destBinPath := filepath.Join(outDir, binName)

	// Check cache first.
	if cacheDir != "" {
		cachedBinPath := filepath.Join(cacheDir, binName)
		if _, err := os.Stat(cachedBinPath); err == nil {
			le.Debug("using cached saucer binary")
			return fsutil.CopyFile(destBinPath, cachedBinPath, 0o755)
		}
	}

	// Download via bun add.
	npmDir := filepath.Join(outDir, "dl-saucer")
	if err := fsutil.CleanCreateDir(npmDir); err != nil {
		return err
	}

	// Create an empty package.json to prevent bun from traversing up.
	pkgJsonPath := filepath.Join(npmDir, "package.json")
	if err := os.WriteFile(pkgJsonPath, []byte("{}"), 0o644); err != nil {
		return err
	}

	le.WithField("npm-pkg", npmPkg).Info("downloading saucer binary via npm")
	cmd, err := npm.BunAdd(ctx, le, stateDir, "--cwd", npmDir, npmPkg)
	if err != nil {
		return err
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return fmt.Errorf("bun add %s failed: %w", npmPkg, err)
	}

	// Find the binary in node_modules.
	npmBinPath := findSaucerBinary(npmDir, platform)
	if npmBinPath == "" {
		return fmt.Errorf("saucer binary not found in %s after npm install", npmPkg)
	}

	// Copy to output directory.
	if err := fsutil.CopyFile(destBinPath, npmBinPath, 0o755); err != nil {
		return fmt.Errorf("failed to copy saucer binary: %w", err)
	}

	// Cache the binary.
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			le.WithError(err).Warn("failed to create saucer cache directory")
		}
		cachedBinPath := filepath.Join(cacheDir, binName)
		if err := fsutil.CopyFile(cachedBinPath, destBinPath, 0o755); err != nil {
			le.WithError(err).Warn("failed to cache saucer binary")
		}
	}

	// Clean up npm download directory.
	_ = os.RemoveAll(npmDir)

	le.Debug("successfully resolved saucer binary")
	return nil
}

// findSaucerBinary looks for the saucer binary in node_modules after npm install.
func findSaucerBinary(npmDir string, platform bldr_platform.Platform) string {
	binName := GetSaucerBinName(platform)
	nodeModules := filepath.Join(npmDir, "node_modules", "@aptre")

	// Try platform-specific package first: @aptre/bldr-saucer-{os}-{arch}/bin/bldr-saucer
	np := bldr_platform.ToNativePlatform(platform)
	if np != nil {
		platformPkg := getSaucerPlatformPkgName(np)
		path := filepath.Join(nodeModules, platformPkg, "bin", binName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try source build fallback: @aptre/bldr-saucer/build/bldr-saucer
	path := filepath.Join(nodeModules, "bldr-saucer", "build", binName)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}

// getSaucerPlatformPkgName returns the npm platform-specific package name.
func getSaucerPlatformPkgName(np *bldr_platform.NativePlatform) string {
	goos := np.GetGOOS()
	goarch := np.GetGOARCH()

	// Map Go os names to npm conventions.
	npmOs := goos
	if goos == "windows" {
		npmOs = "win32"
	}

	// Map Go arch names to npm conventions.
	npmArch := goarch
	if goarch == "amd64" {
		npmArch = "x64"
	}

	return "bldr-saucer-" + npmOs + "-" + npmArch
}

// GetSaucerBinName returns the name of the saucer binary.
func GetSaucerBinName(plat bldr_platform.Platform) string {
	np, ok := plat.(*bldr_platform.NativePlatform)
	if !ok {
		return "bldr-saucer"
	}
	switch np.GetGOOS() {
	case "windows":
		return "bldr-saucer.exe"
	default:
		return "bldr-saucer"
	}
}
