//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr "github.com/s4wave/spacewave/bldr"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

func TestBuildRendererBundleUsesSelfContainedDistEntrypoint(t *testing.T) {
	// Prepare a downstream application with self-contained build sources.
	appRoot, distRoot := setupBundleTestApp(t)

	// Build the Electron preload script from the downstream sources.
	buildDir := filepath.Join(appRoot, "build")
	stateDir := t.TempDir()
	if err := BuildPreloadBundle(context.Background(), logrus.NewEntry(logrus.New()), stateDir, distRoot, buildDir, false, true); err != nil {
		t.Fatal(err)
	}

	// Build the Electron main script from the downstream sources.
	if err := BuildMainBundle(context.Background(), logrus.NewEntry(logrus.New()), stateDir, distRoot, buildDir, false, true); err != nil {
		t.Fatal(err)
	}

	// Build the renderer with its emitted worker module paths.
	if err := BuildRendererBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		stateDir,
		distRoot,
		buildDir,
		"",
		"sw.mjs",
		"shw.mjs",
		"",
		false,
		true,
	); err != nil {
		t.Fatal(err)
	}

	// Read the renderer output for import validation.
	out, err := os.ReadFile(filepath.Join(buildDir, "entrypoint", "entrypoint.mjs"))
	if err != nil {
		t.Fatal(err)
	}

	// Publish stable boot metadata and verify the release manifest exists.
	if err := WriteElectronStableBootFiles(buildDir, "sw.mjs", "shw.mjs"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(buildDir, "browser-release.json")); err != nil {
		t.Fatal(err)
	}

	// Verify that the renderer has no unresolved downstream application imports.
	for _, unexpected := range []string{
		"@s4wave/web/router/app-path.js",
		"@s4wave/app/prerender/boot-status.js",
	} {
		if strings.Contains(string(out), unexpected) {
			t.Fatalf("output still contains unresolved import %q", unexpected)
		}
	}
}

func TestBuildElectronRendererIndexUsesStableBoot(t *testing.T) {
	// Prepare the renderer index destination and package import map.
	dir := t.TempDir()
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react": "/entrypoint/react/index.mjs",
		},
	}

	// Write the renderer index with the supplied package imports.
	if err := BuildElectronRendererIndex(dir, importMap); err != nil {
		t.Fatal(err)
	}

	// Read the generated renderer index.
	out, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the renderer index loads the stable boot module.
	html := string(out)
	if !strings.Contains(html, `<script type="module" src="./boot.mjs"></script>`) {
		t.Fatalf("renderer index missing stable boot path: %s", html)
	}
	if strings.Contains(html, `src="./entrypoint/entrypoint.mjs"`) {
		t.Fatalf("renderer index bypassed stable boot: %s", html)
	}
}

func TestWriteElectronStableBootFiles(t *testing.T) {
	// Prepare the renderer asset that stable boot metadata describes.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "entrypoint"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint", "entrypoint.mjs"), []byte("export default null"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish the stable boot loader and worker metadata.
	if err := WriteElectronStableBootFiles(dir, "sw-electron.mjs", "shw-electron.mjs"); err != nil {
		t.Fatal(err)
	}

	// Read the stable boot loader for startup validation.
	boot, err := os.ReadFile(filepath.Join(dir, "boot.mjs"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that stable boot resets historical state before starting the renderer.
	bootScript := string(boot)
	for _, want := range []string{
		"spacewave-browser-app-state-version",
		"resetHistoricalStateForBoot",
		".then(function(resetStarted){if(!resetStarted)startBoot()})",
	} {
		if !strings.Contains(bootScript, want) {
			t.Fatalf("electron stable boot missing %q: %s", want, bootScript)
		}
	}

	// Read the release manifest that describes the renderer and workers.
	release, err := os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the renderer size, worker paths, and automatic startup metadata.
	releaseJSON := string(release)
	for _, want := range []string{
		`"entrypoint":"entrypoint/entrypoint.mjs"`,
		`"entrypointDecompressedSize":19`,
		`"wasm":"entrypoint/entrypoint.mjs"`,
		`"serviceWorker":"sw-electron.mjs"`,
		`"sharedWorker":"shw-electron.mjs"`,
		`"autoStart":true`,
	} {
		if !strings.Contains(releaseJSON, want) {
			t.Fatalf("electron boot release missing %q: %s", want, releaseJSON)
		}
	}
}

func setupBundleTestApp(t *testing.T) (string, string) {
	// Prepare a downstream module and TypeScript paths for bundle generation.
	t.Helper()
	appRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(appRoot, "go.mod"),
		[]byte("module github.com/example/downstream\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(appRoot, "tsconfig.json"),
		[]byte(`{"compilerOptions":{"paths":{"@go/*":["./vendor/*"]}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Copy the repository distribution sources into the downstream application.
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(testDir, "../../../../.."))
	distRoot := filepath.Join(appRoot, ".bldr", "src")
	if err := bldr.SyncDistSources(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldr.DistSourceSyncConfig{
			RepoRoot:    repoRoot,
			DistRoot:    distRoot,
			BldrSrcPath: repoRoot,
		},
	); err != nil {
		t.Fatal(err)
	}

	return appRoot, distRoot
}
