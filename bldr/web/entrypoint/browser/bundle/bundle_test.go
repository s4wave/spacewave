//go:build !js

package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

func testBldrRoot(t *testing.T) string {
	// Find the package directory used to locate the Bldr source root.
	t.Helper()
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(testDir, "../../../.."))
	if _, err := os.Stat(filepath.Join(root, "web", "entrypoint", "entrypoint.tsx")); err != nil {
		t.Fatalf("Bldr source root %s: %v", root, err)
	}
	return root
}

func testBuildLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(logger)
}

func TestRendererProjectRoot(t *testing.T) {
	// Verify renderer root selection for enclosing and flattened distributions.
	projectRoot := t.TempDir()
	bldrDistRoot := filepath.Join(projectRoot, "bldr")
	if err := os.MkdirAll(bldrDistRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/example/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := rendererProjectRoot(bldrDistRoot); got != projectRoot {
		t.Fatalf("rendererProjectRoot() = %q, want %q", got, projectRoot)
	}
	if err := os.WriteFile(filepath.Join(bldrDistRoot, "go.mod"), []byte("module github.com/example/dist\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := rendererProjectRoot(bldrDistRoot); got != bldrDistRoot {
		t.Fatalf("flattened rendererProjectRoot() = %q, want %q", got, bldrDistRoot)
	}
}

func TestBrowserWorkerRequestPolicy(t *testing.T) {
	// Check the output policies selected for service and shared workers.
	root := testBldrRoot(t)
	buildDir := t.TempDir()
	service := browserScriptRequest(root, buildDir, serviceWorkerSpec(true, true, false))
	if service.GetFormat() != "iife" || service.GetSourcemap() != "inline" {
		t.Fatalf("service worker policy format=%q sourcemap=%q", service.GetFormat(), service.GetSourcemap())
	}
	if service.GetEntryFileNames() != "sw-[hash].mjs" || service.GetDefines()["BLDR_DEBUG"] != "false" {
		t.Fatalf("service worker naming/defines = %q %v", service.GetEntryFileNames(), service.GetDefines())
	}
	shared := browserScriptRequest(root, buildDir, sharedWorkerSpec(false, false, true))
	if shared.GetFormat() != "es" || shared.GetEntryFileNames() != "shw.mjs" {
		t.Fatalf("shared worker policy format=%q name=%q", shared.GetFormat(), shared.GetEntryFileNames())
	}
}

func TestBrowserWorkersBuildDistributedEntrypoints(t *testing.T) {
	// Build each distributed worker entrypoint and verify its result.
	root := testBldrRoot(t)
	for _, test := range []struct {
		name string
		spec browserScriptSpec
	}{
		{"service", serviceWorkerSpec(false, false, true)},
		{"shared", sharedWorkerSpec(false, false, true)},
		{"opfs", opfsWorkerSpec(false, false, true)},
	} {
		t.Run(test.name, func(t *testing.T) {
			buildDir := t.TempDir()
			_, result, err := buildWorkerBundle(
				context.Background(),
				logrus.NewEntry(logrus.New()),
				buildDir,
				root,
				buildDir,
				test.spec,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.GetInputs()) == 0 || len(result.GetOutputs()) == 0 {
				t.Fatalf("incomplete worker result: %s", result.String())
			}
		})
	}
}

func writeForeignRendererOutput(t *testing.T, buildDir string) string {
	// Create the renderer output owned by the foreign build stage.
	t.Helper()
	path := filepath.Join(buildDir, "entrypoint", "pkgs", "runtime", "index.mjs")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreign-output"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireForeignRendererOutput(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foreign-output" {
		t.Fatalf("foreign renderer output changed: %q", data)
	}
}

func TestBuildRendererBuildsDistributedEntrypoint(t *testing.T) {
	// Prepare isolated renderer state and preserve a foreign worker output.
	root := testBldrRoot(t)
	stateDir := t.TempDir()
	buildDir := filepath.Join(t.TempDir(), "build")
	foreignPath := writeForeignRendererOutput(t, buildDir)

	// Build the distributed entrypoint with browser-specific definitions.
	result, err := BuildRenderer(
		context.Background(),
		testBuildLogger(),
		stateDir,
		root,
		buildDir,
		ConfigFreeRendererOpts{
			OutputDir:  filepath.Join(buildDir, "entrypoint"),
			PublicPath: "/entrypoint/",
			Defines: map[string]string{
				"BLDR_IS_BROWSER": "true",
				"BLDR_DEBUG":      "false",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Validate renderer paths and ensure its output was produced.
	if result.JSPath != filepath.Join("entrypoint", "entrypoint.mjs") {
		t.Fatalf("renderer JS path = %q", result.JSPath)
	}
	if len(result.InputFiles) == 0 || len(result.OutputFiles) == 0 {
		t.Fatalf("incomplete renderer result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(buildDir, result.JSPath)); err != nil {
		t.Fatal(err)
	}

	// Ensure renderer output does not overwrite the foreign worker asset.
	requireForeignRendererOutput(t, foreignPath)
}

func TestBuildRendererRoutesCSSGraphThroughVite(t *testing.T) {
	// Prepare renderer inputs and preserve a foreign worker output.
	root := testBldrRoot(t)
	buildDir := filepath.Join(t.TempDir(), "build")
	stateDir := t.TempDir()
	foreignPath := writeForeignRendererOutput(t, buildDir)

	// Build the renderer with its application startup entrypoint.
	result, err := BuildRenderer(
		context.Background(),
		testBuildLogger(),
		stateDir,
		root,
		buildDir,
		ConfigFreeRendererOpts{
			OutputDir:  filepath.Join(buildDir, "entrypoint"),
			PublicPath: "/entrypoint/",
			Defines: map[string]string{
				"BLDR_IS_BROWSER": "true",
				"BLDR_DEBUG":      "false",
				"BLDR_STARTUP_JS": `"../devtool-status/startup.tsx"`,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the CSS graph to produce stylesheets in the build directory.
	if len(result.CSSPaths) == 0 {
		t.Fatalf("CSS-bearing renderer produced no CSS outputs: %+v", result)
	}
	for _, cssPath := range result.CSSPaths {
		if _, err := os.Stat(filepath.Join(buildDir, cssPath)); err != nil {
			t.Fatal(err)
		}
	}

	// Ensure renderer output does not overwrite the foreign worker asset.
	requireForeignRendererOutput(t, foreignPath)
}

func TestWriteBuildManifestIncludesServiceWorker(t *testing.T) {
	// Verify service-worker and optional asset metadata in both release manifests.
	dir := t.TempDir()
	manifest := &BuildManifest{
		Entrypoint:                 "entrypoint/abc123/entrypoint.mjs",
		EntrypointDecompressedSize: 14004885,
		ServiceWorker:              "sw-deadbeef.mjs",
		SharedWorker:               "shw-beadfeed.mjs",
		OpfsWorker:                 "opfs-worker-c0ffee.mjs",
		Wasm:                       "entrypoint/abc123/runtime.wasm",
		CSS:                        []string{"static/app.css"},
		DefaultManifestBundle: &DefaultManifestBundle{
			Metadata: "/manifest-pack.json",
			Pack:     "/manifest.pack.kvf",
		},
	}

	// Write both release manifests before validating their contents.
	if err := WriteBuildManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	// Read the application manifest before parsing its fields.
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Parse the application manifest before validating its output fields.
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// Check service-worker and core asset metadata in the application manifest.
	if got := string(v.GetStringBytes("serviceWorker")); got != manifest.ServiceWorker {
		t.Fatalf("unexpected serviceWorker: %q", got)
	}
	if got := string(v.GetStringBytes("entrypoint")); got != manifest.Entrypoint {
		t.Fatalf("unexpected entrypoint: %q", got)
	}
	if got := v.GetInt("entrypointDecompressedSize"); got != int(manifest.EntrypointDecompressedSize) {
		t.Fatalf("unexpected entrypointDecompressedSize: %d", got)
	}
	if got := string(v.GetStringBytes("wasm")); got != manifest.Wasm {
		t.Fatalf("unexpected wasm: %q", got)
	}

	// Verify the OPFS worker is included in the application manifest.
	if got := string(v.GetStringBytes("opfsWorker")); got != manifest.OpfsWorker {
		t.Fatalf("unexpected opfsWorker: %q", got)
	}

	// Read the release manifest and verify its shell assets.
	data, err = os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Parse the release manifest before validating its schema.
	v, err = p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// Validate release schema and embedded service-worker assets.
	if got := v.GetInt("schemaVersion"); got != 1 {
		t.Fatalf("unexpected schemaVersion: %d", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "serviceWorker")); got != manifest.ServiceWorker {
		t.Fatalf("unexpected release serviceWorker: %q", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "entrypoint")); got != manifest.Entrypoint {
		t.Fatalf("unexpected release entrypoint: %q", got)
	}
	if got := v.GetInt("shellAssets", "entrypointDecompressedSize"); got != int(manifest.EntrypointDecompressedSize) {
		t.Fatalf("unexpected release entrypointDecompressedSize: %d", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "wasm")); got != manifest.Wasm {
		t.Fatalf("unexpected release wasm: %q", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "opfsWorker")); got != manifest.OpfsWorker {
		t.Fatalf("unexpected release opfsWorker: %q", got)
	}

	// Verify the default manifest bundle metadata and pack paths.
	if got := string(v.GetStringBytes("defaultManifestBundle", "metadata")); got != manifest.DefaultManifestBundle.Metadata {
		t.Fatalf("unexpected release default bundle metadata: %q", got)
	}
	if got := string(v.GetStringBytes("defaultManifestBundle", "pack")); got != manifest.DefaultManifestBundle.Pack {
		t.Fatalf("unexpected release default bundle pack: %q", got)
	}
}

// TestWriteBuildManifestGenerationTracksContent covers a release that changes
// only the embedded assets: the service worker filename stays the same, so the
// generation must come from content or clients keep the old release cached.
func TestWriteBuildManifestGenerationTracksContent(t *testing.T) {
	// Verify embedded asset bytes determine the release generation.
	dir := t.TempDir()
	entryDir := filepath.Join(dir, "entrypoint", "abc123")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &BuildManifest{
		Entrypoint:    "entrypoint/abc123/entrypoint.mjs",
		ServiceWorker: "sw-deadbeef.mjs",
		SharedWorker:  "shw-beadfeed.mjs",
	}

	// Generate manifests for repeated and changed embedded asset content.
	generation := func(kvfile string) string {
		// Write an asset payload and generate its release manifest.
		t.Helper()
		if err := os.WriteFile(filepath.Join(entryDir, "assets.kvfile"), []byte(kvfile), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteBuildManifest(dir, manifest); err != nil {
			t.Fatal(err)
		}

		// Read the generated release manifest before parsing its fields.
		data, err := os.ReadFile(filepath.Join(dir, "browser-release.json"))
		if err != nil {
			t.Fatal(err)
		}

		// Parse the generated release manifest before extracting its identity.
		var p fastjson.Parser
		v, err := p.ParseBytes(data)
		if err != nil {
			t.Fatal(err)
		}

		// Return the generated release identity.
		return string(v.GetStringBytes("generationId"))
	}

	// Compare generations for unchanged and changed embedded asset content.
	first := generation("old plugin")
	if first == "" || first == manifest.ServiceWorker {
		t.Fatalf("expected a content generation, got %q", first)
	}
	if again := generation("old plugin"); again != first {
		t.Fatalf("expected a stable generation, got %q then %q", first, again)
	}
	if changed := generation("new plugin"); changed == first {
		t.Fatalf("expected a new generation after the kvfile changed, got %q", changed)
	}
}

func TestWriteBuildManifestOmitsOptionalWasm(t *testing.T) {
	// Ensure manifests omit optional wasm assets when none are built.
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

	// Read the application manifest before parsing its optional fields.
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Parse the application manifest before checking its wasm field.
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the application manifest omits wasm.
	if got := v.Get("wasm"); got != nil {
		t.Fatalf("unexpected manifest wasm: %s", got)
	}

	// Read the release manifest before checking its shell assets.
	data, err = os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Parse the release manifest before checking its wasm field.
	v, err = p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the release manifest omits wasm.
	if got := v.Get("shellAssets", "wasm"); got != nil {
		t.Fatalf("unexpected release wasm: %s", got)
	}
}

func TestWriteStableBootAsset(t *testing.T) {
	// Check the stable browser boot script preserves its startup contract.
	dir := t.TempDir()
	if err := WriteStableBootAsset(dir); err != nil {
		t.Fatal(err)
	}

	// Read the generated boot script before checking its startup behavior.
	data, err := os.ReadFile(filepath.Join(dir, stableBootFilename))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)

	// Verify stable release lookup and versioned browser state keys.
	if !strings.Contains(script, "/browser-release.json") {
		t.Fatalf("boot asset missing stable release manifest path: %s", script)
	}
	if !strings.Contains(script, "bootStateVersion='1000001'") {
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

	// Check reset registries and their ownership classifications.
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

	// Check worker, cache, and targeted storage cleanup operations.
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

	// Expose release identity and boot lifecycle marks to diagnostics.
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

	// Include recovery status details in boot diagnostics.
	if !strings.Contains(script, "__swBootRecoveryStatus") ||
		!strings.Contains(script, "compatibilityVersion:bootStateVersion") ||
		!strings.Contains(script, "lastResetDecision") {
		t.Fatalf("boot asset missing boot recovery status fields: %s", script)
	}

	// Expose startup progress and phase ordering to the boot page.
	if !strings.Contains(script, "data-sw-boot-progress") {
		t.Fatalf("boot asset missing progress target support: %s", script)
	}
	if !strings.Contains(script, "startupPhaseOrder") {
		t.Fatalf("boot asset missing startup phase rail ordering: %s", script)
	}
	if !strings.Contains(script, "data-sw-boot-phase") {
		t.Fatalf("boot asset missing startup phase rail target support: %s", script)
	}

	// Protect prerendered documents from runtime status mutations.
	if !strings.Contains(script, "canMutateBootStatusTarget") {
		t.Fatalf("boot asset missing prerender mutation guard: %s", script)
	}
	if !strings.Contains(script, "#bldr-root[data-prerendered]") {
		t.Fatalf("boot asset missing prerender root selector: %s", script)
	}
	if !strings.Contains(script, "#sw-loading") {
		t.Fatalf("boot asset missing root loading selector: %s", script)
	}
	if !strings.Contains(script, "data-sw-boot-visibility") {
		t.Fatalf("boot asset missing root visibility attribute: %s", script)
	}

	// Preserve static handoff metadata and quickstart link rewriting.
	if !strings.Contains(script, "__swStaticHandoffLinks") {
		t.Fatalf("boot asset missing static handoff link flag: %s", script)
	}
	if !strings.Contains(script, `a[href^="/quickstart/"]`) {
		t.Fatalf("boot asset missing static quickstart link rewrite selector: %s", script)
	}
}

func TestBuildRendererIndexUsesEntrypointPath(t *testing.T) {
	// Verify the renderer index uses its explicit module entrypoint.
	dir := t.TempDir()
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react": "/entrypoint/react/index.mjs",
		},
	}
	if err := BuildRendererIndex(dir, "./entrypoint/entrypoint.mjs", importMap); err != nil {
		t.Fatal(err)
	}

	// Read the generated index before checking its document links.
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

// TestWriteBuildManifestOfflineRuntimeAssets verifies the offline inventory
// includes the pack and lazy modules while excluding source maps.
func TestWriteBuildManifestOfflineRuntimeAssets(t *testing.T) {
	// Prepare runtime assets and manifests for an offline inventory.
	dir := t.TempDir()
	root := filepath.Join(dir, "entrypoint", "test", "chunks")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"entrypoint/test/assets.kvfile", "entrypoint/test/runtime-goscript.mjs", "entrypoint/test/chunks/lazy.js", "entrypoint/test/chunks/lazy.js.map"} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte("asset"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteBuildManifest(dir, &BuildManifest{}); err != nil {
		t.Fatal(err)
	}

	// Parse each manifest and verify its required static assets.
	for _, name := range []string{"manifest.json", "browser-release.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var parser fastjson.Parser
		value, err := parser.ParseBytes(data)
		if err != nil {
			t.Fatal(err)
		}
		assets := value.GetArray("requiredStaticAssets")
		if len(assets) != 3 {
			t.Fatalf("%s: expected pack, runtime, and lazy module; got %s", name, data)
		}
		for idx, want := range []string{"/entrypoint/test/assets.kvfile", "/entrypoint/test/chunks/lazy.js", "/entrypoint/test/runtime-goscript.mjs"} {
			if string(assets[idx].GetStringBytes()) != want {
				t.Fatalf("%s: missing %s", name, want)
			}
		}
	}
}
