//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr "github.com/s4wave/spacewave/bldr"
	web_entrypoint_index "github.com/s4wave/spacewave/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

func TestBuildRendererBundleUsesSelfContainedDistEntrypoint(t *testing.T) {
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

	distRoot := filepath.Join(appRoot, ".bldr", "src")
	copyDistSources(t, distRoot)

	buildDir := filepath.Join(appRoot, "build")
	if err := BuildRendererBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
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

	out, err := os.ReadFile(filepath.Join(buildDir, "entrypoint", "entrypoint.mjs"))
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

func TestBuildElectronRendererIndexUsesStableBoot(t *testing.T) {
	dir := t.TempDir()
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react": "/entrypoint/react/index.mjs",
		},
	}
	if err := BuildElectronRendererIndex(dir, importMap); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, `<script type="module" src="./boot.mjs"></script>`) {
		t.Fatalf("renderer index missing stable boot path: %s", html)
	}
	if strings.Contains(html, `src="./entrypoint/entrypoint.mjs"`) {
		t.Fatalf("renderer index bypassed stable boot: %s", html)
	}
}

func TestWriteElectronStableBootFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "entrypoint"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint", "entrypoint.mjs"), []byte("export default null"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteElectronStableBootFiles(dir, "sw-electron.mjs", "shw-electron.mjs"); err != nil {
		t.Fatal(err)
	}

	boot, err := os.ReadFile(filepath.Join(dir, "boot.mjs"))
	if err != nil {
		t.Fatal(err)
	}
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

	release, err := os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}
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

func copyDistSources(t *testing.T, dest string) {
	t.Helper()
	if err := fs.WalkDir(bldr.DistSources, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := bldr.DistSources.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
}
