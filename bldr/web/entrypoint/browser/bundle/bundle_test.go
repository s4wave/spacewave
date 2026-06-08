//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
)

func TestBrowserBuildOptsResolvesGoVendorImportsFromNestedDir(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(projectRoot, "tsconfig.json"),
		[]byte(`{"compilerOptions":{"paths":{"@go/*":["./vendor/*"]}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "global.d.ts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	vendorDir := filepath.Join(projectRoot, "vendor", "example")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(vendorDir, "mod.ts"),
		[]byte(`export const greeting = "hello"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	workingDir := filepath.Join(projectRoot, "web", "entrypoint", "browser")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(workingDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { greeting } from "@go/example/mod.js"; console.log(greeting);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(projectRoot, "out.js")
	opts := BrowserBuildOpts(workingDir, false, true)
	opts.EntryPoints = []string{"entry.ts"}
	opts.Outfile = outFile
	opts.Write = true

	result := esbuild.Build(opts)
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
}

func TestBrowserEntrypointBuildOptsBuildsDistributedEntrypoint(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bldrRoot := filepath.Clean(filepath.Join(testDir, "../../../.."))
	if _, err := os.Stat(filepath.Join(bldrRoot, "web", "entrypoint", "entrypoint.tsx")); os.IsNotExist(err) {
		t.Skipf("skipping: bldr entrypoint not found under %s", bldrRoot)
	}

	outDir := t.TempDir()
	opts := BrowserEntrypointBuildOpts(bldrRoot, false, false)
	opts.Outdir = outDir
	opts.Write = true

	result := esbuild.Build(opts)
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(filepath.Join(outDir, "entrypoint.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{
		"@s4wave/web/router/app-path.js",
		"@s4wave/app/prerender/boot-status.js",
	} {
		if strings.Contains(string(out), unexpected) {
			t.Fatalf("output still contains unresolved import %q", unexpected)
		}
	}
}

func TestServiceWorkerBuildOptsBuildsClassicScript(t *testing.T) {
	opts := ServiceWorkerBuildOpts(t.TempDir(), false, true, true)
	if opts.Format != esbuild.FormatIIFE {
		t.Fatalf("service worker format=%v want %v", opts.Format, esbuild.FormatIIFE)
	}
}

func TestRuntimeDistDepsResolverPinsQuickJSWASIReactor(t *testing.T) {
	projectRoot := t.TempDir()
	pkgDir := filepath.Join(projectRoot, "state", "build-web-pkgs", "node_modules", "quickjs-wasi-reactor", "dist")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(`export const marker = "bldr-dist-dep";`), 0o644); err != nil {
		t.Fatal(err)
	}

	workingDir := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(workingDir, "entry.ts"),
		[]byte(`import { marker } from "quickjs-wasi-reactor"; console.log(marker);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	opts := BrowserBuildOpts(workingDir, false, false)
	ApplyRuntimeDistDepsResolver(&opts, filepath.Join(projectRoot, "state", "build-web-pkgs"))
	opts.EntryPoints = []string{"entry.ts"}
	opts.Outfile = filepath.Join(projectRoot, "out.js")
	opts.Write = true

	result := esbuild.Build(opts)
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(opts.Outfile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "bldr-dist-dep") {
		t.Fatalf("output does not contain dist dependency marker: %s", out)
	}
}

func TestBrowserBuildOptsAppliesReadableJavaScriptPolicy(t *testing.T) {
	readable := BrowserBuildOpts(t.TempDir(), false, false)
	if readable.MinifyWhitespace || readable.MinifyIdentifiers || readable.MinifySyntax {
		t.Fatalf("readable opts minified: whitespace=%v identifiers=%v syntax=%v", readable.MinifyWhitespace, readable.MinifyIdentifiers, readable.MinifySyntax)
	}
	if readable.Sourcemap != esbuild.SourceMapNone {
		t.Fatalf("readable opts sourcemap=%v want none", readable.Sourcemap)
	}
	if readable.TreeShaking != esbuild.TreeShakingTrue {
		t.Fatalf("readable opts tree shaking=%v want true", readable.TreeShaking)
	}

	minifiedWithMaps := BrowserBuildOpts(t.TempDir(), true, true)
	if !minifiedWithMaps.MinifyWhitespace || !minifiedWithMaps.MinifyIdentifiers || !minifiedWithMaps.MinifySyntax {
		t.Fatalf("minified opts not fully minified: whitespace=%v identifiers=%v syntax=%v", minifiedWithMaps.MinifyWhitespace, minifiedWithMaps.MinifyIdentifiers, minifiedWithMaps.MinifySyntax)
	}
	if minifiedWithMaps.Sourcemap != esbuild.SourceMapLinked {
		t.Fatalf("minified opts sourcemap=%v want linked", minifiedWithMaps.Sourcemap)
	}
	if minifiedWithMaps.TreeShaking != esbuild.TreeShakingTrue {
		t.Fatalf("minified opts tree shaking=%v want true", minifiedWithMaps.TreeShaking)
	}
}

func TestApplyTinyGoNodeFallbacks(t *testing.T) {
	opts := BrowserBuildOpts(t.TempDir(), false, true)
	ApplyTinyGoNodeFallbacks(&opts)

	for _, module := range []string{
		"fs",
		"crypto",
		"util",
		"node:fs",
		"node:crypto",
		"node:util",
	} {
		if !slices.Contains(opts.External, module) {
			t.Fatalf("missing TinyGo external %q in %v", module, opts.External)
		}
	}
}

func TestWriteBuildManifestIncludesServiceWorker(t *testing.T) {
	dir := t.TempDir()
	manifest := &BuildManifest{
		Entrypoint:    "entrypoint/abc123/entrypoint.mjs",
		ServiceWorker: "sw-deadbeef.mjs",
		SharedWorker:  "shw-beadfeed.mjs",
		Wasm:          "entrypoint/abc123/runtime.wasm",
		CSS:           []string{"static/app.css"},
	}
	if err := WriteBuildManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(v.GetStringBytes("serviceWorker")); got != manifest.ServiceWorker {
		t.Fatalf("unexpected serviceWorker: %q", got)
	}
	if got := string(v.GetStringBytes("entrypoint")); got != manifest.Entrypoint {
		t.Fatalf("unexpected entrypoint: %q", got)
	}
	if got := string(v.GetStringBytes("wasm")); got != manifest.Wasm {
		t.Fatalf("unexpected wasm: %q", got)
	}

	data, err = os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}
	v, err = p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.GetInt("schemaVersion"); got != 1 {
		t.Fatalf("unexpected schemaVersion: %d", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "serviceWorker")); got != manifest.ServiceWorker {
		t.Fatalf("unexpected release serviceWorker: %q", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "entrypoint")); got != manifest.Entrypoint {
		t.Fatalf("unexpected release entrypoint: %q", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "wasm")); got != manifest.Wasm {
		t.Fatalf("unexpected release wasm: %q", got)
	}
}

func TestWriteBuildManifestOmitsOptionalWasm(t *testing.T) {
	dir := t.TempDir()
	manifest := &BuildManifest{
		Entrypoint:    "entrypoint/abc123/entrypoint.mjs",
		ServiceWorker: "sw-deadbeef.mjs",
		SharedWorker:  "shw-beadfeed.mjs",
		CSS:           []string{"static/app.css"},
	}
	if err := WriteBuildManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("wasm"); got != nil {
		t.Fatalf("unexpected manifest wasm: %s", got)
	}

	data, err = os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}
	v, err = p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("shellAssets", "wasm"); got != nil {
		t.Fatalf("unexpected release wasm: %s", got)
	}
}

func TestWriteStableBootAsset(t *testing.T) {
	dir := t.TempDir()
	if err := WriteStableBootAsset(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, stableBootFilename))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "/browser-release.json") {
		t.Fatalf("boot asset missing stable release manifest path: %s", script)
	}
	if !strings.Contains(script, "bootStateVersion='1000000'") {
		t.Fatalf("boot asset missing major browser app state version: %s", script)
	}
	if !strings.Contains(script, "spacewave-browser-app-state-version") {
		t.Fatalf("boot asset missing durable browser app state version key: %s", script)
	}
	if !strings.Contains(script, "spacewave-browser-tab-state-version") {
		t.Fatalf("boot asset missing per-tab browser app state version key: %s", script)
	}
	if !strings.Contains(script, "spacewave-browser-app-state-reset-attempted") {
		t.Fatalf("boot asset missing reset attempt guard key: %s", script)
	}
	if !strings.Contains(script, "bootLocalStorageKeys") {
		t.Fatalf("boot asset missing localStorage shell key list: %s", script)
	}
	if !strings.Contains(script, "bootSessionStorageKeys") {
		t.Fatalf("boot asset missing sessionStorage shell key list: %s", script)
	}
	if !strings.Contains(script, "bootStorageResetRules") {
		t.Fatalf("boot asset missing storage reset classification registry: %s", script)
	}
	if !strings.Contains(script, "owner:'web-state-atom'") ||
		!strings.Contains(script, "key:'app-persistent'") ||
		!strings.Contains(script, "resetPolicy:'preserve'") {
		t.Fatalf("boot asset missing app-persistent preserve classification: %s", script)
	}
	if !strings.Contains(script, "owner:'shell-tab-state-atom'") ||
		!strings.Contains(script, "key:'tab-state-'") ||
		!strings.Contains(script, "call-site-audit-required-before-reset'") {
		t.Fatalf("boot asset missing tab-state preserve classification: %s", script)
	}
	if !strings.Contains(script, "owner:'auth-handoff-flow'") ||
		!strings.Contains(script, "key:'spacewave-auth-handoff-payload'") ||
		!strings.Contains(script, "auth-flow-owner-reset-only'") {
		t.Fatalf("boot asset missing auth handoff preserve classification: %s", script)
	}
	if !strings.Contains(script, "navigator.serviceWorker.getRegistrations") {
		t.Fatalf("boot asset missing ServiceWorker registration reset: %s", script)
	}
	if !strings.Contains(script, "g.caches.keys") {
		t.Fatalf("boot asset missing CacheStorage reset: %s", script)
	}
	if !strings.Contains(script, "storageRemoveKnown(localStorage,bootLocalStorageKeys,bootLocalStoragePrefixes)") {
		t.Fatalf("boot asset missing targeted localStorage shell reset: %s", script)
	}
	if !strings.Contains(script, "storageRemoveKnown(sessionStorage,bootSessionStorageKeys,[])") {
		t.Fatalf("boot asset missing targeted sessionStorage shell reset: %s", script)
	}
	if !strings.Contains(script, "if(settledAllFulfilled(cleanupResults)){") ||
		!strings.Contains(script, "storageSet(localStorage,bootStateVersionKey,bootStateVersion)") {
		t.Fatalf("boot asset must record durable version only after cleanup succeeds: %s", script)
	}
	if !strings.Contains(script, ".then(function(resetStarted){if(!resetStarted)startBoot()})") {
		t.Fatalf("boot asset must reset historical state before starting boot: %s", script)
	}
	if !strings.Contains(script, "__swGenerationId") {
		t.Fatalf("boot asset missing generation exposure: %s", script)
	}
	if !strings.Contains(script, "__swServiceWorker") {
		t.Fatalf("boot asset missing service worker exposure: %s", script)
	}
	if !strings.Contains(script, "spacewave:boot-status") {
		t.Fatalf("boot asset missing boot status event: %s", script)
	}
	if !strings.Contains(script, "spacewave.startup.") {
		t.Fatalf("boot asset missing startup mark prefix: %s", script)
	}
	if !strings.Contains(script, "boot-status.") {
		t.Fatalf("boot asset missing boot status mark labels: %s", script)
	}
	if !strings.Contains(script, "__swBootStatus") {
		t.Fatalf("boot asset missing boot status global: %s", script)
	}
	if !strings.Contains(script, "__swBootRecoveryStatus") ||
		!strings.Contains(script, "compatibilityVersion:bootStateVersion") ||
		!strings.Contains(script, "lastResetDecision") {
		t.Fatalf("boot asset missing boot recovery status fields: %s", script)
	}
	if !strings.Contains(script, "data-sw-boot-progress") {
		t.Fatalf("boot asset missing progress target support: %s", script)
	}
	if !strings.Contains(script, "startupPhaseOrder") {
		t.Fatalf("boot asset missing startup phase rail ordering: %s", script)
	}
	if !strings.Contains(script, "data-sw-boot-phase") {
		t.Fatalf("boot asset missing startup phase rail target support: %s", script)
	}
	if !strings.Contains(script, "canMutateBootStatusTarget") {
		t.Fatalf("boot asset missing prerender mutation guard: %s", script)
	}
	if !strings.Contains(script, "#bldr-root[data-prerendered]") {
		t.Fatalf("boot asset missing prerender root selector: %s", script)
	}
	if !strings.Contains(script, "#sw-loading") {
		t.Fatalf("boot asset missing root loading selector: %s", script)
	}
	if !strings.Contains(script, "__swStaticHandoffLinks") {
		t.Fatalf("boot asset missing static handoff link flag: %s", script)
	}
	if !strings.Contains(script, `a[href^="/quickstart/"]`) {
		t.Fatalf("boot asset missing static quickstart link rewrite selector: %s", script)
	}
}

func TestBuildRendererIndexUsesEntrypointPath(t *testing.T) {
	dir := t.TempDir()
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react": "/entrypoint/react/index.mjs",
		},
	}
	if err := BuildRendererIndex(dir, "./entrypoint/entrypoint.mjs", importMap); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if !strings.Contains(html, `<script type="module" src="./entrypoint/entrypoint.mjs"></script>`) {
		t.Fatalf("renderer index missing explicit entrypoint path: %s", html)
	}
	if strings.Contains(html, "./boot.mjs") {
		t.Fatalf("renderer index unexpectedly referenced boot.mjs: %s", html)
	}
}
