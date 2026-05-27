//go:build !js

package devtool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/fastjson"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
)

func TestWriteWebWsBuildManifest(t *testing.T) {
	dir := t.TempDir()
	err := writeWebWsBuildManifest(dir, &entrypoint_browser_bundle.BrowserBundleResult{
		EntrypointPath:        "entrypoint/entrypoint.mjs",
		ServiceWorkerFilename: "sw.mjs",
		SharedWorkerFilename:  "shw.mjs",
		CSSPaths:              []string{"entrypoint/app.css"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(v.GetStringBytes("shellAssets", "entrypoint")); got != "entrypoint/entrypoint.mjs" {
		t.Fatalf("unexpected entrypoint: %q", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "wasm")); got != "entrypoint/runtime-ws.mjs" {
		t.Fatalf("unexpected websocket runtime asset: %q", got)
	}

	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}
