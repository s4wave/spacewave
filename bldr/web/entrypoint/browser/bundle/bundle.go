//go:build !js

package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/bldr/util/npm"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	web_pkg_vite "github.com/s4wave/spacewave/bldr/web/pkg/vite"
	"github.com/sirupsen/logrus"
)

// BrowserBundleResult contains the output filenames from a browser bundle build.
type BrowserBundleResult struct {
	// EntrypointPath is the path to the entrypoint mjs relative to the build dir.
	EntrypointPath string
	// ServiceWorkerFilename is the output filename of the service worker.
	ServiceWorkerFilename string
	// SharedWorkerFilename is the output filename of the shared worker.
	SharedWorkerFilename string
	// CSSPaths contains CSS output file paths relative to the build dir.
	CSSPaths []string
}

// BuildManifest is the manifest.json structure written alongside index.html.
// The prerender build script reads this to discover asset URLs.
type BuildManifest struct {
	Entrypoint    string   `json:"entrypoint"`
	ServiceWorker string   `json:"serviceWorker"`
	SharedWorker  string   `json:"sharedWorker"`
	Wasm          string   `json:"wasm"`
	CSS           []string `json:"css"`
}

const stableBootFilename = "boot.mjs"

// WriteBuildManifest writes a manifest.json to the given directory.
func WriteBuildManifest(dir string, manifest *BuildManifest) error {
	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("entrypoint", a.NewString(manifest.Entrypoint))
	obj.Set("serviceWorker", a.NewString(manifest.ServiceWorker))
	obj.Set("sharedWorker", a.NewString(manifest.SharedWorker))
	obj.Set("wasm", a.NewString(manifest.Wasm))
	css := a.NewArray()
	for _, path := range manifest.CSS {
		css.SetArrayItem(len(css.GetArray()), a.NewString(path))
	}
	obj.Set("css", css)
	data := obj.MarshalTo(nil)
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
}

// WriteStableBootAsset writes the stable browser boot asset at the build root.
func WriteStableBootAsset(dir string) error {
	const bootAsset = `const releasePath='/browser-release.json';
const g=globalThis;
let releasePromise;
let primePromise;
function absPath(path){
  if(!path)return'';
  return path.startsWith('/')?path:'/'+path;
}
function loadRelease(){
  if(releasePromise)return releasePromise;
  releasePromise=fetch(releasePath,{cache:'no-cache'}).then(async function(resp){
    if(!resp.ok)throw new Error('failed to load browser release manifest: '+resp.status);
    const release=await resp.json();
    const shellAssets=release.shellAssets||{};
    const entrypoint=absPath(shellAssets.entrypoint);
    const wasm=absPath(shellAssets.wasm);
    if(!entrypoint)throw new Error('browser release manifest missing shellAssets.entrypoint');
    if(!wasm)throw new Error('browser release manifest missing shellAssets.wasm');
    g.__swEntry=entrypoint;
    g.__swGenerationId=release.generationId||'';
    return {entrypoint,wasm};
  });
  return releasePromise;
}
function primeRelease(){
  if(primePromise)return primePromise;
  primePromise=loadRelease().then(function(release){
    fetch(release.wasm);
    return release;
  });
  return primePromise;
}
(function(){
  let readyResolve;
  g.__swReady=new Promise(function(resolve){readyResolve=resolve});
  g.__swReadyResolve=readyResolve;
  g.__swDeferBoot=true;
  let imported=false;
  function doImport(){
    if(imported)return;
    imported=true;
    void primeRelease()
      .then(function(release){return import(release.entrypoint)})
      .catch(function(err){console.error('boot.mjs: failed to import entrypoint',err)});
  }
  void primeRelease()
    .then(function(release){
      if(window.location.hash.length>1||localStorage.getItem('spacewave-has-session')){
        const landing=document.getElementById('sw-landing');
        const loading=document.getElementById('sw-loading');
        if(landing)landing.style.display='none';
        if(loading)loading.style.display='';
        doImport();
        return;
      }
      fetch(release.entrypoint);
      function onInteract(){
        doImport();
        document.removeEventListener('click',onInteract);
        document.removeEventListener('scroll',onInteract);
        document.removeEventListener('keydown',onInteract);
      }
      document.addEventListener('click',onInteract);
      document.addEventListener('scroll',onInteract,{passive:true});
      document.addEventListener('keydown',onInteract);
      window.addEventListener('load',function(){setTimeout(doImport,1000)});
    })
    .catch(function(err){console.error('boot.mjs: failed to load release manifest',err)});
})();`

	return os.WriteFile(filepath.Join(dir, stableBootFilename), []byte(bootAsset), 0o644)
}

// EsbuildLogLevel is the log level when bundling the bundle.
var EsbuildLogLevel = esbuild.LogLevelWarning

// DefaultBanner is the default banner applied to code files.
func DefaultBanner() map[string]string {
	return map[string]string{
		"js": "// © 2018-2025 Aperture Robotics, LLC. <support@aperture.us>\n// All rights reserved.",
	}
}

func resolveBrowserBuildRoot(workingDir string) string {
	dir := workingDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "tsconfig.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return workingDir
		}
		dir = parent
	}
}

// BrowserBuildOpts are general options for building for the browser.
func BrowserBuildOpts(workingDir string, minify bool) esbuild.BuildOptions {
	sourceMap := esbuild.SourceMapNone
	if !minify {
		sourceMap = esbuild.SourceMapLinked
	}

	var drop esbuild.Drop
	if minify {
		drop = esbuild.DropDebugger
	}

	projectRoot := resolveBrowserBuildRoot(workingDir)

	return esbuild.BuildOptions{
		AbsWorkingDir: workingDir,

		Target:      esbuild.ES2024,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformBrowser,
		LogLevel:    EsbuildLogLevel,
		TreeShaking: esbuild.TreeShakingTrue,
		Sourcemap:   sourceMap,
		Drop:        drop,

		Metafile:  false,
		Splitting: false,

		Banner: DefaultBanner(),
		Define: map[string]string{
			"BLDR_IS_BROWSER": "true",
		},
		Plugins: []esbuild.Plugin{
			bldr_esbuild_build.GoVendorTsResolverPlugin(projectRoot),
		},

		Loader: map[string]esbuild.Loader{
			".wasm":  esbuild.LoaderFile,
			".woff":  esbuild.LoaderFile,
			".woff2": esbuild.LoaderFile,
			".png":   esbuild.LoaderFile,
			".jpg":   esbuild.LoaderFile,
			".jpeg":  esbuild.LoaderFile,
			".svg":   esbuild.LoaderFile,
			".gif":   esbuild.LoaderFile,
		},
		OutExtension: map[string]string{
			".js": ".mjs",
		},

		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,

		Bundle: true,
	}
}

// BrowserEntrypointBuildOpts creates the BuildOpts for the root browser entrypoint
func BrowserEntrypointBuildOpts(bldrDistRoot string, minify bool) esbuild.BuildOptions {
	buildOpts := BrowserBuildOpts(bldrDistRoot, minify)
	buildOpts.External = slices.Clone(web_pkg_external.BldrExternal)
	buildOpts.External = append(buildOpts.External, "tailwindcss")
	buildOpts.EntryPointsAdvanced = []esbuild.EntryPoint{{
		InputPath:  "web/entrypoint/entrypoint.tsx",
		OutputPath: "entrypoint",
	}}
	return buildOpts
}

// ServiceWorkerBuildOpts creates the BuildOpts for the service worker
func ServiceWorkerBuildOpts(bldrDistRoot string, minify, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify)
	if hash {
		baseConfig.EntryNames = "sw-[hash]"
	} else {
		baseConfig.EntryNames = "sw"
	}
	baseConfig.EntryPoints = []string{"web/bldr/service-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// SharedWorkerBuildOpts creates the BuildOpts for the shared worker
func SharedWorkerBuildOpts(bldrDistRoot string, minify, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify)
	if hash {
		baseConfig.EntryNames = "shw-[hash]"
	} else {
		baseConfig.EntryNames = "shw"
	}
	baseConfig.EntryPoints = []string{"web/bldr/shared-worker.ts"}
	baseConfig.EntryPointsAdvanced = nil
	return baseConfig
}

// BuildServiceWorkerBundle builds specifically the service worker files.
//
// Returns the filename of the service worker output file (including the hash).
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) (string, error) {
	le.Debug("generating service-worker bundle")

	swOpts := ServiceWorkerBuildOpts(bldrDistRoot, minify, !devMode)
	swOpts.Outdir = buildDir
	swOpts.Write = true
	if !minify {
		swOpts.Sourcemap = esbuild.SourceMapInline
	}
	swOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)
	result := esbuild.Build(swOpts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return "", err
	}
	if len(result.OutputFiles) != 1 {
		return "", errors.Errorf("expected %d output files but got %d", 1, len(result.OutputFiles))
	}
	return filepath.Base(result.OutputFiles[0].Path), nil
}

// BuildSharedWorkerBundle builds specifically the shared worker files.
//
// Returns the filename of the shared worker output file (including the hash).
func BuildSharedWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, devMode bool) (string, error) {
	le.Debug("generating shared-worker bundle")

	shwOpts := SharedWorkerBuildOpts(bldrDistRoot, minify, !devMode)
	shwOpts.Outdir = buildDir
	shwOpts.Write = true
	if !minify {
		shwOpts.Sourcemap = esbuild.SourceMapInline
	}
	shwOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)
	result := esbuild.Build(shwOpts)
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return "", err
	}
	for _, f := range result.OutputFiles {
		if strings.HasSuffix(f.Path, ".mjs") {
			return filepath.Base(f.Path), nil
		}
	}
	return "", errors.New("shared worker build produced no .mjs output")
}

// BuildRendererIndex builds the web renderer index.html.
//
// importMap contains the web pkg import map entries (from BuildWebPkgsBundle).
func BuildRendererIndex(buildDir, entrypointPath string, importMap web_entrypoint_index.ImportMap) error {
	// render index.html
	indexHtml, err := web_entrypoint_index.RenderIndexHTML(web_entrypoint_index.IndexData{
		ImportMap:      importMap,
		EntrypointPath: entrypointPath,
	})
	if err != nil {
		return err
	}
	rendererHtmlOut := filepath.Join(buildDir, "index.html")
	return os.WriteFile(rendererHtmlOut, []byte(indexHtml), 0o644)
}

// BuildRendererBundle builds the web renderer bundle files.
//
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
// BuildRendererBundle builds the web renderer bundle and returns CSS output
// paths relative to buildDir.
func BuildRendererBundle(
	le *logrus.Entry,
	sourcesRoot,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath,
	entrypointHash string,
	minify,
	forceDedicatedWorkers,
	devMode bool,
	webPkgImportMap web_entrypoint_index.ImportMap,
) ([]string, error) {
	le.Debug("generating web renderer bundle")

	if err := BuildRendererIndex(buildDir, "./"+stableBootFilename, webPkgImportMap); err != nil {
		return nil, err
	}

	// entrypoint
	webEntrypointOut := filepath.Join(buildDir, "entrypoint")
	if entrypointHash != "" {
		webEntrypointOut = filepath.Join(webEntrypointOut, entrypointHash)
	}

	rendererBuildOpts := BrowserEntrypointBuildOpts(bldrDistRoot, minify)
	rendererBuildOpts.Outdir = webEntrypointOut
	rendererBuildOpts.Write = true

	// Set PublicPath so esbuild emits correct URLs for file-loader assets
	// (images, wasm, fonts). Assets are output to the entrypoint dir which is
	// served at /entrypoint/ or /entrypoint/{hash}/.
	assetPublicPath := "/entrypoint/"
	if entrypointHash != "" {
		assetPublicPath = "/entrypoint/" + entrypointHash + "/"
	}
	rendererBuildOpts.PublicPath = assetPublicPath

	if runtimeJsPath != "" {
		rendererBuildOpts.Define["BLDR_RUNTIME_JS"] = strconv.Quote(runtimeJsPath)
	}

	if runtimeSwPath != "" {
		rendererBuildOpts.Define["BLDR_SW_JS"] = strconv.Quote(runtimeSwPath)
	}

	if runtimeShwPath != "" {
		rendererBuildOpts.Define["BLDR_SHW_JS"] = strconv.Quote(runtimeShwPath)
	}

	distSourcesDirToSourcesRoot, err := filepath.Rel(bldrDistRoot, sourcesRoot)
	if err != nil {
		return nil, err
	}

	if webStartupSrcPath != "" {
		// esbuild interprets this path in an import() statement
		// we need a relative path from the entrypoint.tsx to the src path.
		// add an extra .. for the "web/entrypoint"
		webStartupSrcPathRel := filepath.Join(distSourcesDirToSourcesRoot, "../..", webStartupSrcPath)
		rendererBuildOpts.Define["BLDR_STARTUP_JS"] = strconv.Quote(webStartupSrcPathRel)
	}

	rendererBuildOpts.Define["BLDR_DEBUG"] = strconv.FormatBool(devMode)

	if forceDedicatedWorkers {
		rendererBuildOpts.Define["BLDR_FORCE_DEDICATED_WORKERS"] = "true"
	}

	if !minify {
		rendererBuildOpts.Sourcemap = esbuild.SourceMapLinked
	}

	res := esbuild.Build(rendererBuildOpts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return nil, err
	}

	// collect CSS output paths relative to buildDir
	var cssPaths []string
	for _, f := range res.OutputFiles {
		if strings.HasSuffix(f.Path, ".css") {
			rel, relErr := filepath.Rel(buildDir, f.Path)
			if relErr == nil {
				cssPaths = append(cssPaths, rel)
			}
		}
	}
	return cssPaths, nil
}

// BuildBrowserBundle builds and outputs the web & service worker files.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// webStartupSrcPath is the path to the startup js module to load for the react app entrypoint (can be empty).
// entrypointHash, if set, builds into /entrypoint/{entrypointHash}/...
func BuildBrowserBundle(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	sourcesRoot,
	bldrDistRoot,
	buildDir,
	runtimeJsPath,
	runtimeSwPath,
	runtimeShwPath,
	webStartupSrcPath string,
	entrypointHash string,
	minify,
	devMode,
	forceDedicatedWorkers bool,
) (*BrowserBundleResult, error) {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return nil, err
	}

	// service worker
	swFilename, err := BuildServiceWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return nil, err
	}

	// shared worker
	shwFilename, err := BuildSharedWorkerBundle(le, bldrDistRoot, buildDir, minify, devMode)
	if err != nil {
		return nil, err
	}

	// replace the filename in runtimeSwPath with the sw filename
	runtimeSwPath = filepath.Join(filepath.Dir(runtimeSwPath), swFilename)
	// replace the filename in runtimeShwPath with the shw filename
	runtimeShwPath = filepath.Join(filepath.Dir(runtimeShwPath), shwFilename)

	// web pkgs
	// use platform for linux -> node.js (react and react-dom don't care.)
	bldrNativePlatform, err := bldr_platform.ParseNativePlatform("desktop/linux/amd64")
	if err != nil {
		return nil, err
	}

	pkgsPathPrefix := "/entrypoint"
	if entrypointHash != "" {
		pkgsPathPrefix += "/" + entrypointHash
	}

	entrypointDir := filepath.Join(buildDir, "entrypoint")
	if entrypointHash != "" {
		entrypointDir = filepath.Join(entrypointDir, entrypointHash)
	}

	webPkgImportMap, err := BuildWebPkgsBundle(ctx, le, stateDir, bldrNativePlatform, bldrDistRoot, entrypointDir, pkgsPathPrefix, minify, devMode)
	if err != nil {
		return nil, err
	}

	// renderer bundle
	cssPaths, err := BuildRendererBundle(le, sourcesRoot, bldrDistRoot, buildDir, runtimeJsPath, runtimeSwPath, runtimeShwPath, webStartupSrcPath, entrypointHash, minify, forceDedicatedWorkers, devMode, webPkgImportMap)
	if err != nil {
		return nil, err
	}
	if err := WriteStableBootAsset(buildDir); err != nil {
		return nil, err
	}

	// build the entrypoint path relative to the build dir
	entrypointPath := "entrypoint"
	if entrypointHash != "" {
		entrypointPath += "/" + entrypointHash
	}
	entrypointPath += "/entrypoint.mjs"

	return &BrowserBundleResult{
		EntrypointPath:        entrypointPath,
		ServiceWorkerFilename: swFilename,
		SharedWorkerFilename:  shwFilename,
		CSSPaths:              cssPaths,
	}, nil
}

// BuildWebPkgsBundle builds the web pkg bundle files.
//
// stateDir is the directory where bun will be downloaded if not found in PATH.
// pathPrefix is the prefix to prepend to /pkgs/ for pkg paths
// Returns the import map entries mapping logical specifiers to hashed output paths.
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, stateDir string, plat bldr_platform.Platform, bldrDistRoot, buildDir, pathPrefix string, minify, devMode bool) (web_entrypoint_index.ImportMap, error) {
	// build to pkgs/
	outDir := filepath.Join(buildDir, "pkgs")

	// install dist deps (cached: skips if package.json unchanged)
	// Use stateDir (not buildDir) so the cache survives CleanCreateDir on the build output.
	buildPkgsDir, _ := filepath.Abs(filepath.Join(stateDir, "build-web-pkgs"))
	if err := npm.EnsureBunInstall(ctx, le, stateDir, bldr.ResolveDistSourcePath(bldrDistRoot, "dist", "deps", "package.json"), buildPkgsDir); err != nil {
		return web_entrypoint_index.ImportMap{}, err
	}

	// web pkgs we distribute with bldr
	refs := web_pkg_external.GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot)

	// if we are in development mode: include test-utils to react-dom
	if devMode {
		for _, ref := range refs {
			if ref.WebPkgId == "react-dom" {
				ref.Imports = append(ref.Imports, "test-utils.js")
			}
		}
	}

	var importMap web_entrypoint_index.ImportMap
	viteWorkingPath := filepath.Join(stateDir, "vite-web-pkgs")
	err := web_pkg_vite.RunOneShot(ctx, le, bldrDistRoot, bldrDistRoot, viteWorkingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		_, _, mapEntries, buildErr := web_pkg_vite.BuildWebPkgsVite(
			ctx,
			le,
			buildDir,
			refs,
			outDir,
			pathPrefix+"/pkgs/",
			minify,
			client,
			filepath.Join(viteWorkingPath, "cache"),
		)
		if buildErr == nil {
			importMap = web_pkg_vite.BuildImportMapFromEntries(mapEntries)
		}
		return buildErr
	})
	return importMap, err
}
