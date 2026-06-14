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
	Wasm          string   `json:"wasm,omitempty"`
	CSS           []string `json:"css"`
	AutoStart     bool     `json:"autoStart,omitempty"`
}

const stableBootFilename = "boot.mjs"

// WriteBuildManifest writes a manifest.json to the given directory.
func WriteBuildManifest(dir string, manifest *BuildManifest) error {
	if err := writeBrowserReleaseManifest(dir, manifest); err != nil {
		return err
	}

	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("entrypoint", a.NewString(manifest.Entrypoint))
	obj.Set("serviceWorker", a.NewString(manifest.ServiceWorker))
	obj.Set("sharedWorker", a.NewString(manifest.SharedWorker))
	if manifest.Wasm != "" {
		obj.Set("wasm", a.NewString(manifest.Wasm))
	}
	css := a.NewArray()
	for _, path := range manifest.CSS {
		css.SetArrayItem(len(css.GetArray()), a.NewString(path))
	}
	obj.Set("css", css)
	data := obj.MarshalTo(nil)
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
}

func writeBrowserReleaseManifest(dir string, manifest *BuildManifest) error {
	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("schemaVersion", a.NewNumberInt(1))
	obj.Set("generationId", a.NewString(manifest.ServiceWorker))
	if manifest.AutoStart {
		obj.Set("autoStart", a.NewTrue())
	}

	shellAssets := a.NewObject()
	shellAssets.Set("entrypoint", a.NewString(manifest.Entrypoint))
	shellAssets.Set("serviceWorker", a.NewString(manifest.ServiceWorker))
	shellAssets.Set("sharedWorker", a.NewString(manifest.SharedWorker))
	if manifest.Wasm != "" {
		shellAssets.Set("wasm", a.NewString(manifest.Wasm))
	}
	css := a.NewArray()
	for _, path := range manifest.CSS {
		css.SetArrayItem(len(css.GetArray()), a.NewString(path))
	}
	shellAssets.Set("css", css)
	obj.Set("shellAssets", shellAssets)

	routes := a.NewArray()
	routes.SetArrayItem(0, a.NewString("/"))
	obj.Set("prerenderedRoutes", routes)
	obj.Set("requiredStaticAssets", a.NewArray())

	data := obj.MarshalTo(nil)
	return os.WriteFile(filepath.Join(dir, "browser-release.json"), data, 0o644)
}

// WriteStableBootAsset writes the stable browser boot asset at the build root.
func WriteStableBootAsset(dir string) error {
	const bootAsset = `const releasePath='/browser-release.json';
const bootStateVersion='1000000';
const bootStateVersionKey='spacewave-browser-app-state-version';
const bootSessionStateVersionKey='spacewave-browser-tab-state-version';
const bootStateResetAttemptKey='spacewave-browser-app-state-reset-attempted';
const g=globalThis;
const bootStorageResetRules=[
  {area:'localStorage',kind:'key',key:'spacewave-has-session',owner:'browser-boot-session-hint',durability:'derived-shell-hint',resetPolicy:'reset',migrationPolicy:'recompute-from-session-list'},
  {area:'localStorage',kind:'key',key:'spacewave-has-interacted',owner:'landing-ui',durability:'derived-ui-hint',resetPolicy:'reset',migrationPolicy:'recompute-from-interaction'},
  {area:'localStorage',kind:'key',key:'spacewave-state-devtools',owner:'devtools-ui',durability:'developer-ui-preference',resetPolicy:'reset',migrationPolicy:'recreate-default-devtools-state'},
  {area:'localStorage',kind:'key',key:'spacewave-devtools-state',owner:'devtools-ui',durability:'developer-ui-preference',resetPolicy:'reset',migrationPolicy:'recreate-default-devtools-state'},
  {area:'localStorage',kind:'key',key:'app-persistent',owner:'web-state-atom',durability:'unknown-persistent-state',resetPolicy:'preserve',migrationPolicy:'owner-audit-required-before-reset'},
  {area:'localStorage',kind:'prefix',key:'tab-state-',owner:'shell-tab-state-atom',durability:'unknown-tab-scoped-state',resetPolicy:'preserve',migrationPolicy:'call-site-audit-required-before-reset'},
  {area:'sessionStorage',kind:'key',key:'shell-tabs-state',owner:'shell-tabs-ui',durability:'session-ui-state',resetPolicy:'reset',migrationPolicy:'recreate-default-home-tab'},
  {area:'sessionStorage',kind:'key',key:'shell-tabs-layout',owner:'shell-tabs-ui',durability:'session-ui-state',resetPolicy:'reset',migrationPolicy:'recreate-default-layout'},
  {area:'sessionStorage',kind:'key',key:'spacewave-sso-start-provider',owner:'sso-start-flow',durability:'transient-auth-workflow-state',resetPolicy:'preserve',migrationPolicy:'auth-flow-owner-reset-only'},
  {area:'sessionStorage',kind:'key',key:'spacewave-sso-return-to',owner:'sso-start-flow',durability:'transient-auth-workflow-state',resetPolicy:'preserve',migrationPolicy:'auth-flow-owner-reset-only'},
  {area:'sessionStorage',kind:'key',key:'spacewave-pending-join',owner:'join-flow',durability:'transient-join-workflow-state',resetPolicy:'preserve',migrationPolicy:'join-flow-owner-reset-only'},
  {area:'sessionStorage',kind:'key',key:'spacewave-auth-handoff-payload',owner:'auth-handoff-flow',durability:'transient-auth-workflow-state',resetPolicy:'preserve',migrationPolicy:'auth-flow-owner-reset-only'}
];
g.__swBootStorageResetRules=bootStorageResetRules.map(function(rule){return Object.assign({},rule)});
function storageResetKeys(area,kind){
  return bootStorageResetRules
    .filter(function(rule){return rule.area===area&&rule.kind===kind&&rule.resetPolicy==='reset'})
    .map(function(rule){return rule.key});
}
const bootLocalStorageKeys=storageResetKeys('localStorage','key');
const bootLocalStoragePrefixes=storageResetKeys('localStorage','prefix');
const bootSessionStorageKeys=storageResetKeys('sessionStorage','key');
const bootStatusEvent='spacewave:boot-status';
const startupMarkPrefix='spacewave.startup.';
const startupMarkEvent='spacewave-startup-mark';
let bootLastResetDecision='unknown';
let releasePromise;
let primePromise;
let nextStartupMarkSequence=1;
const phaseProgress={loading:.04,manifest:.12,'manifest-ready':.22,wasm:.38,entrypoint:.54,runtime:.76,ready:.9,app:.96};
const startupPhaseInfo={
  prepare:{label:'Prepare',detail:'Preparing browser files.',progress:.08},
  connect:{label:'Connect',detail:'Connecting the app shell.',progress:.3},
  runtime:{label:'Runtime',detail:'Starting the Spacewave runtime.',progress:.58},
  frame:{label:'App',detail:'Downloading the app bundle. This can take a while the first time.',progress:.84},
  done:{label:'Done',detail:'Spacewave is ready.',progress:1}
};
const startupPhaseOrder=['prepare','connect','runtime','frame','done'];
const bootPhaseStartupPhase={loading:'prepare',manifest:'prepare','manifest-ready':'prepare','manifest-error':'prepare',wasm:'connect',entrypoint:'connect','entrypoint-error':'connect',runtime:'runtime',ready:'runtime','runtime-error':'runtime',app:'frame'};
function startupDisplayForBootPhase(phase,state){
  const id=bootPhaseStartupPhase[phase]||'prepare';
  const info=startupPhaseInfo[id];
  return {id:id,detail:info.label+': '+info.detail,progress:info.progress,indeterminate:id==='frame',error:state==='error'};
}
function markStartupBoundary(label,detail){
  const name=startupMarkPrefix+label;
  const sequence=g.__swStartupMarkSequence??nextStartupMarkSequence;
  g.__swStartupMarkSequence=sequence+1;
  nextStartupMarkSequence=sequence+1;
  const markDetail=Object.assign({},detail||{},{label:label,sequence:sequence});
  g.__swStartupMarks=(g.__swStartupMarks||[]).concat([{name:name,label:label,sequence:sequence,detail:markDetail}]);
  if(g.performance&&typeof g.performance.mark==='function'){
    try{g.performance.mark(name,{detail:markDetail})}catch(_){g.performance.mark(name)}
  }
  window.dispatchEvent(new CustomEvent(startupMarkEvent,{detail:{name:name,detail:markDetail}}));
  return name;
}
function setBootStatus(phase,detail,state){
  const progress=phaseProgress[phase];
  const status={phase,detail:detail||phase,state:state||'loading',compatibilityVersion:bootStateVersion,lastResetDecision:bootLastResetDecision};
  const display=startupDisplayForBootPhase(phase,status.state);
  if(progress!==undefined)status.progress=progress;
  g.__swBootStatus=status;
  const target=document.querySelector('[data-sw-boot-status]');
  if(canMutateBootStatusTarget(target))target.textContent=display.detail;
  const stateTarget=document.querySelector('[data-sw-boot-state]');
  if(canMutateBootStatusTarget(stateTarget))stateTarget.setAttribute('data-sw-boot-state',status.state);
  if(display.progress!==undefined){
    const pct=Math.round(display.progress*100);
    const progressTarget=document.querySelector('[data-sw-boot-progress]');
    if(canMutateBootStatusTarget(progressTarget)){
      progressTarget.style.width=display.indeterminate?'33%':pct+'%';
      progressTarget.style.transition=display.indeterminate?'none':'width 200ms';
      progressTarget.classList.toggle('animate-progress-indeterminate',!!display.indeterminate);
      if(display.indeterminate){
        progressTarget.removeAttribute('aria-valuenow');
        progressTarget.setAttribute('aria-valuetext','Loading');
      }else{
        progressTarget.removeAttribute('aria-valuetext');
        progressTarget.setAttribute('aria-valuenow',String(pct));
      }
    }
    const progressLabel=document.querySelector('[data-sw-boot-progress-label]');
    if(canMutateBootStatusTarget(progressLabel))progressLabel.textContent=display.indeterminate?'':pct+'%';
  }
  updateStaticPhaseRail(display.id,status.state);
  markStartupBoundary('boot-status.'+phase,{source:'boot',phase:phase,state:status.state,progress:status.progress});
  window.dispatchEvent(new CustomEvent(bootStatusEvent,{detail:status}));
}
function setBootResetDecision(decision,detail){
  bootLastResetDecision=decision;
  g.__swBootRecoveryStatus={compatibilityVersion:bootStateVersion,lastResetDecision:decision,detail:detail||''};
  if(g.__swBootStatus)g.__swBootStatus=Object.assign({},g.__swBootStatus,{compatibilityVersion:bootStateVersion,lastResetDecision:decision});
}
function updateStaticPhaseRail(currentID,bootState){
  const currentIdx=startupPhaseOrder.indexOf(currentID);
  for(let i=0;i<startupPhaseOrder.length;i++){
    const phaseID=startupPhaseOrder[i];
    const target=document.querySelector('[data-sw-boot-phase="'+phaseID+'"]');
    if(!canMutateBootStatusTarget(target))continue;
    const phaseState=i<currentIdx?'complete':i===currentIdx&&bootState==='error'?'error':i===currentIdx?'current':'pending';
    target.setAttribute('data-sw-boot-phase-state',phaseState);
    const dot=target.querySelector('[data-sw-boot-phase-dot]');
    if(dot)dot.style.background=phaseState==='error'?'var(--color-destructive,#ef4444)':phaseState==='pending'?'color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)':'var(--color-brand,var(--color-logo-blue,#4f8cff))';
    const label=target.querySelector('[data-sw-boot-phase-label]');
    if(label)label.style.color=phaseState==='error'?'var(--color-destructive,#ef4444)':phaseState==='current'?'var(--color-foreground,#fafafa)':phaseState==='complete'?'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)':'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)';
  }
}
function canMutateBootStatusTarget(target){
  if(!target)return false;
  const root=target.closest('#bldr-root[data-prerendered]');
  if(!root)return true;
  return !!target.closest('#sw-loading');
}
function rewriteStaticHandoffLinks(){
  g.__swStaticHandoffLinks=true;
  const landing=document.getElementById('sw-landing');
  if(!landing)return;
  for(const link of landing.querySelectorAll('a[href^="/quickstart/"]')){
    const href=link.getAttribute('href');
    if(href)link.setAttribute('href','#'+href);
  }
}
function setBootError(phase,err){
  const msg=err&&err.message?err.message:String(err);
  setBootStatus(phase,msg,'error');
}
function storageGet(storage,key){
  try{return storage&&storage.getItem?storage.getItem(key):null}catch(_){return null}
}
function storageSet(storage,key,value){
  try{if(storage&&storage.setItem)storage.setItem(key,value)}catch(_){}
}
function storageRemove(storage,key){
  try{if(storage&&storage.removeItem)storage.removeItem(key)}catch(_){}
}
function storageRemoveKnown(storage,keys,prefixes){
  try{
    if(!storage||!storage.removeItem)return;
    for(const key of keys)storage.removeItem(key);
    if(!prefixes||!storage.key)return;
    const matched=[];
    for(let i=0;i<storage.length;i++){
      const key=storage.key(i);
      if(key&&prefixes.some(function(prefix){return key.startsWith(prefix)}))matched.push(key);
    }
    for(const key of matched)storage.removeItem(key);
  }catch(_){}
}
async function unregisterServiceWorkersForBootReset(){
  if(!navigator.serviceWorker||typeof navigator.serviceWorker.getRegistrations!=='function')return;
  const registrations=await navigator.serviceWorker.getRegistrations();
  await Promise.all(registrations.map(function(registration){return registration.unregister()}));
}
async function clearCachesForBootReset(){
  if(!g.caches||typeof g.caches.keys!=='function')return;
  const cacheNames=await g.caches.keys();
  await Promise.all(cacheNames.map(function(cacheName){return g.caches.delete(cacheName)}));
}
function reloadAfterBootStateReset(){
  try{window.location.reload()}catch(_){}
  try{
    const next=new URL(window.location.href);
    next.searchParams.set('bootResetReload',String(Date.now()));
    window.location.replace(next.toString());
    return;
  }catch(_){}
  try{window.location.href=window.location.href}catch(_){}
}
function settledAllFulfilled(results){
  return results.every(function(result){return result.status==='fulfilled'});
}
function clearBootSessionState(){
  storageRemoveKnown(sessionStorage,bootSessionStorageKeys,[]);
  storageSet(sessionStorage,bootSessionStateVersionKey,bootStateVersion);
}
async function resetHistoricalStateForBoot(){
  const storedVersion=storageGet(localStorage,bootStateVersionKey);
  if(storedVersion===bootStateVersion){
    if(storageGet(sessionStorage,bootSessionStateVersionKey)!==bootStateVersion){
      clearBootSessionState();
      setBootResetDecision('tab-session-state-reset','tab session version mismatch');
    }else{
      setBootResetDecision('current','stored compatibility version current');
    }
    storageRemove(sessionStorage,bootStateResetAttemptKey);
    return false;
  }
  if(storageGet(sessionStorage,bootStateResetAttemptKey)===bootStateVersion){
    clearBootSessionState();
    setBootResetDecision('attempt-guard','reset already attempted in this tab');
    return false;
  }
  setBootResetDecision('reset-started','stored compatibility version mismatch');
  setBootStatus('loading','Updating Spacewave app shell...');
  clearBootSessionState();
  storageRemoveKnown(localStorage,bootLocalStorageKeys,bootLocalStoragePrefixes);
  storageSet(sessionStorage,bootStateResetAttemptKey,bootStateVersion);
  const cleanupResults=await Promise.allSettled([
    unregisterServiceWorkersForBootReset(),
    clearCachesForBootReset()
  ]);
  if(settledAllFulfilled(cleanupResults)){
    storageSet(localStorage,bootStateVersionKey,bootStateVersion);
    setBootResetDecision('reset-complete','shell cleanup completed');
  }else{
    setBootResetDecision('reset-cleanup-failed','shell cleanup did not fully complete');
  }
  reloadAfterBootStateReset();
  return true;
}
function absPath(path){
  if(!path)return'';
  if(/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(path))return path;
  return path.startsWith('/')?path:'/'+path;
}
function loadRelease(){
  if(releasePromise)return releasePromise;
  setBootStatus('manifest','Loading browser release...');
  releasePromise=fetch(releasePath,{cache:'no-cache'}).then(async function(resp){
    if(!resp.ok)throw new Error('failed to load browser release manifest: '+resp.status);
    const release=await resp.json();
    const shellAssets=release.shellAssets||{};
    const entrypoint=absPath(shellAssets.entrypoint);
    const wasm=absPath(shellAssets.wasm);
    const serviceWorker=absPath(shellAssets.serviceWorker);
    if(!entrypoint)throw new Error('browser release manifest missing shellAssets.entrypoint');
    if(!serviceWorker)throw new Error('browser release manifest missing shellAssets.serviceWorker');
    g.__swEntry=entrypoint;
    g.__swServiceWorker=serviceWorker;
    g.__swGenerationId=release.generationId||'';
    setBootStatus('manifest-ready','Browser release found.');
    return {entrypoint,wasm,serviceWorker,autoStart:release.autoStart===true};
  });
  return releasePromise;
}
function primeRelease(){
  if(primePromise)return primePromise;
  primePromise=loadRelease().then(function(release){
    if(release.wasm){
      setBootStatus('wasm','Preparing runtime...');
      fetch(release.wasm);
    }
    return release;
  });
  return primePromise;
}
function startBoot(){
  rewriteStaticHandoffLinks();
  let readyResolve;
  g.__swReady=new Promise(function(resolve){readyResolve=resolve});
  g.__swReadyResolve=readyResolve;
  g.__swDeferBoot=true;
  let imported=false;
  function doImport(){
    if(imported)return;
    imported=true;
    setBootStatus('entrypoint','Starting application...');
    void primeRelease()
      .then(function(release){return import(release.entrypoint)})
      .catch(function(err){setBootError('entrypoint-error',err);console.error('boot.mjs: failed to import entrypoint',err)});
  }
  void primeRelease()
    .then(function(release){
      if(release.autoStart||window.location.hash.length>1||localStorage.getItem('spacewave-has-session')){
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
    .catch(function(err){setBootError('manifest-error',err);console.error('boot.mjs: failed to load release manifest',err)});
}
(function(){
  void resetHistoricalStateForBoot()
    .then(function(resetStarted){if(!resetStarted)startBoot()})
    .catch(function(err){console.error('boot.mjs: failed to reset historical browser state',err);startBoot()});
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

func resolveBrowserDistBuildRoot(workingDir string) string {
	dir := workingDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "global.d.ts")); err == nil {
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
func BrowserBuildOpts(workingDir string, minify, sourcemaps bool) esbuild.BuildOptions {
	sourceMap := esbuild.SourceMapNone
	if sourcemaps {
		sourceMap = esbuild.SourceMapLinked
	}

	var drop esbuild.Drop
	if minify {
		drop = esbuild.DropDebugger
	}

	projectRoot := resolveBrowserBuildRoot(workingDir)
	distRoot := resolveBrowserDistBuildRoot(workingDir)

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
			bldr_esbuild_build.GoVendorTsResolverPlugin(projectRoot, distRoot),
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

// ApplyTinyGoNodeFallbacks keeps TinyGo's browser wasm_exec.js Node fallbacks
// external so the injected browser stubs can provide runtime globals instead.
func ApplyTinyGoNodeFallbacks(opts *esbuild.BuildOptions) {
	opts.External = append(opts.External,
		"fs",
		"crypto",
		"util",
		"node:fs",
		"node:crypto",
		"node:util",
	)
}

// BrowserEntrypointBuildOpts creates the BuildOpts for the root browser entrypoint
func BrowserEntrypointBuildOpts(bldrDistRoot string, minify, sourcemaps bool) esbuild.BuildOptions {
	buildOpts := BrowserBuildOpts(bldrDistRoot, minify, sourcemaps)
	buildOpts.External = slices.Clone(web_pkg_external.BldrExternal)
	buildOpts.External = append(buildOpts.External, "tailwindcss")
	buildOpts.EntryPointsAdvanced = []esbuild.EntryPoint{{
		InputPath:  "web/entrypoint/entrypoint.tsx",
		OutputPath: "entrypoint",
	}}
	return buildOpts
}

const quickJSWASIReactorModule = "quickjs-wasi-reactor"

func RuntimeDistDepsResolverPlugin(buildPkgsDir, bldrDistRoot string) esbuild.Plugin {
	return esbuild.Plugin{
		Name: "bldr-runtime-dist-deps-resolver",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^quickjs-wasi-reactor$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				modulePath := filepath.Join(buildPkgsDir, "node_modules", quickJSWASIReactorModule, "dist", "index.js")
				if _, err := os.Stat(modulePath); err != nil {
					return esbuild.OnResolveResult{}, errors.Wrapf(err, "resolve %s from bldr dist deps", quickJSWASIReactorModule)
				}
				return esbuild.OnResolveResult{Path: modulePath}, nil
			})
			build.OnResolve(esbuild.OnResolveOptions{
				Filter: `^@aptre/bldr(?:-react)?(?:/.*)?$`,
			}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				modulePath, ok := resolveBldrRuntimePackagePath(bldrDistRoot, args.Path)
				if !ok {
					return esbuild.OnResolveResult{}, nil
				}
				if _, err := os.Stat(modulePath); err != nil {
					return esbuild.OnResolveResult{}, errors.Wrapf(err, "resolve %s from bldr dist source", args.Path)
				}
				return esbuild.OnResolveResult{Path: modulePath}, nil
			})
		},
	}
}

func ApplyRuntimeDistDepsResolver(opts *esbuild.BuildOptions, buildPkgsDir string) {
	if buildPkgsDir == "" {
		return
	}
	opts.Plugins = append(opts.Plugins, RuntimeDistDepsResolverPlugin(buildPkgsDir, opts.AbsWorkingDir))
}

func resolveBldrRuntimePackagePath(bldrDistRoot, importPath string) (string, bool) {
	for _, pkg := range []struct {
		id  string
		dir string
	}{
		{id: "@aptre/bldr", dir: filepath.Join("web", "bldr")},
		{id: "@aptre/bldr-react", dir: filepath.Join("web", "bldr-react")},
	} {
		if importPath == pkg.id {
			return filepath.Join(bldrDistRoot, pkg.dir, "index.ts"), true
		}
		if after, ok := strings.CutPrefix(importPath, pkg.id+"/"); ok {
			return filepath.Join(bldrDistRoot, pkg.dir, after), true
		}
	}
	return "", false
}

// ServiceWorkerBuildOpts creates the BuildOpts for the service worker
func ServiceWorkerBuildOpts(bldrDistRoot string, minify, sourcemaps, hash bool) esbuild.BuildOptions {
	return ServiceWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, "", minify, sourcemaps, hash)
}

func ServiceWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, buildPkgsDir string, minify, sourcemaps, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify, sourcemaps)
	ApplyRuntimeDistDepsResolver(&baseConfig, buildPkgsDir)
	baseConfig.Format = esbuild.FormatIIFE
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
func SharedWorkerBuildOpts(bldrDistRoot string, minify, sourcemaps, hash bool) esbuild.BuildOptions {
	return SharedWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, "", minify, sourcemaps, hash)
}

func SharedWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, buildPkgsDir string, minify, sourcemaps, hash bool) esbuild.BuildOptions {
	baseConfig := BrowserBuildOpts(bldrDistRoot, minify, sourcemaps)
	ApplyRuntimeDistDepsResolver(&baseConfig, buildPkgsDir)
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
func BuildServiceWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, sourcemaps, devMode bool) (string, error) {
	return BuildServiceWorkerBundleWithRuntimeDeps(le, bldrDistRoot, buildDir, "", minify, sourcemaps, devMode)
}

func BuildServiceWorkerBundleWithRuntimeDeps(le *logrus.Entry, bldrDistRoot, buildDir, buildPkgsDir string, minify, sourcemaps, devMode bool) (string, error) {
	le.Debug("generating service-worker bundle")

	swOpts := ServiceWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, buildPkgsDir, minify, sourcemaps, !devMode)
	swOpts.Outdir = buildDir
	swOpts.Write = true
	if sourcemaps {
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
func BuildSharedWorkerBundle(le *logrus.Entry, bldrDistRoot, buildDir string, minify, sourcemaps, devMode bool) (string, error) {
	return BuildSharedWorkerBundleWithRuntimeDeps(le, bldrDistRoot, buildDir, "", minify, sourcemaps, devMode)
}

func BuildSharedWorkerBundleWithRuntimeDeps(le *logrus.Entry, bldrDistRoot, buildDir, buildPkgsDir string, minify, sourcemaps, devMode bool) (string, error) {
	le.Debug("generating shared-worker bundle")

	shwOpts := SharedWorkerBuildOptsWithRuntimeDeps(bldrDistRoot, buildPkgsDir, minify, sourcemaps, !devMode)
	shwOpts.Outdir = buildDir
	shwOpts.Write = true
	if sourcemaps {
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
	sourcemaps,
	forceDedicatedWorkers,
	forceMessagePortWorkerComms,
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

	rendererBuildOpts := BrowserEntrypointBuildOpts(bldrDistRoot, minify, sourcemaps)
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
	if forceMessagePortWorkerComms {
		rendererBuildOpts.Define["BLDR_FORCE_MESSAGEPORT_WORKER_COMMS"] = "true"
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
	sourcemaps,
	devMode,
	forceDedicatedWorkers,
	forceMessagePortWorkerComms bool,
) (*BrowserBundleResult, error) {
	err := os.MkdirAll(buildDir, 0o755)
	if err != nil {
		return nil, err
	}

	buildPkgsDir, err := EnsureBldrDistDepsInstall(ctx, le, stateDir, bldrDistRoot)
	if err != nil {
		return nil, err
	}

	// service worker
	swFilename, err := BuildServiceWorkerBundleWithRuntimeDeps(le, bldrDistRoot, buildDir, buildPkgsDir, minify, sourcemaps, devMode)
	if err != nil {
		return nil, err
	}

	// shared worker
	shwFilename, err := BuildSharedWorkerBundleWithRuntimeDeps(le, bldrDistRoot, buildDir, buildPkgsDir, minify, sourcemaps, devMode)
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

	webPkgImportMap, err := BuildWebPkgsBundle(ctx, le, stateDir, bldrNativePlatform, bldrDistRoot, entrypointDir, pkgsPathPrefix, minify, sourcemaps, devMode)
	if err != nil {
		return nil, err
	}

	// renderer bundle
	cssPaths, err := BuildRendererBundle(le, sourcesRoot, bldrDistRoot, buildDir, runtimeJsPath, runtimeSwPath, runtimeShwPath, webStartupSrcPath, entrypointHash, minify, sourcemaps, forceDedicatedWorkers, forceMessagePortWorkerComms, devMode, webPkgImportMap)
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
func BuildWebPkgsBundle(ctx context.Context, le *logrus.Entry, stateDir string, plat bldr_platform.Platform, bldrDistRoot, buildDir, pathPrefix string, minify, sourcemaps, devMode bool) (web_entrypoint_index.ImportMap, error) {
	// build to pkgs/
	outDir := filepath.Join(buildDir, "pkgs")

	// install dist deps (cached: skips if package.json unchanged)
	// Use stateDir (not buildDir) so the cache survives CleanCreateDir on the build output.
	buildPkgsDir, err := EnsureBldrDistDepsInstall(ctx, le, stateDir, bldrDistRoot)
	if err != nil {
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
	err = web_pkg_vite.RunOneShot(ctx, le, bldrDistRoot, bldrDistRoot, viteWorkingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		_, _, mapEntries, buildErr := web_pkg_vite.BuildWebPkgsVite(
			ctx,
			le,
			buildDir,
			refs,
			outDir,
			pathPrefix+"/pkgs/",
			minify,
			minify,
			sourcemaps,
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

func EnsureBldrDistDepsInstall(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot string) (string, error) {
	buildPkgsDir, _ := filepath.Abs(filepath.Join(stateDir, "build-web-pkgs"))
	err := npm.EnsureBunInstall(ctx, le, stateDir, bldr.ResolveDistSourcePath(bldrDistRoot, "dist", "deps", "package.json"), buildPkgsDir)
	if err != nil {
		return "", err
	}
	return buildPkgsDir, nil
}
