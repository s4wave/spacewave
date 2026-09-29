//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

func TestWritePluginHandoffManifestRecordsSurfacesAndPack(t *testing.T) {
	// Stage the pack files and the manifest refs in a handoff root.
	root := t.TempDir()
	manifestRefsPath := filepath.Join(root, "manifest-refs.json")
	writePluginTestFile(t, filepath.Join(root, "manifest.pack.kvf"), []byte("pack"))
	writePluginTestFile(t, filepath.Join(root, "manifest-pack.bin"), []byte("metadata"))
	writePluginTestFile(t, manifestRefsPath, []byte(`[
  {"manifest_id":"devtool","platform_id":"browser/js","rev":31,"ref":"browser-ref"},
  {"manifest_id":"devtool","platform_id":"darwin/arm64","rev":31,"ref":"darwin-ref"}
]`))

	// Write the handoff manifest for the browser and macOS surfaces.
	if err := writePluginHandoffManifest(pluginHandoffOptions{
		rootDir:            root,
		manifestRefsPath:   manifestRefsPath,
		pluginRev:          "abc123",
		releaseEnvironment: "plugin-production",
		requestedSelection: "browser,macos",
		gitSHA:             "abc123",
		runID:              "456",
		runAttempt:         "3",
		sourceRepo:         "s4wave/spacewave",
		workflow:           "plugin-release",
		includeBrowser:     true,
		includeMacOS:       true,
	}); err != nil {
		t.Fatalf("writePluginHandoffManifest() error = %v", err)
	}

	// Parse the written manifest.
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	// Require the provenance, surfaces, and refs it was given.
	if got := string(v.GetStringBytes("run_id")); got != "456" {
		t.Fatalf("run_id = %q, want 456", got)
	}
	if !v.GetBool("produced_surfaces", "browser") || !v.GetBool("produced_surfaces", "macos") {
		t.Fatalf("browser/macos surfaces not recorded: %s", data)
	}
	if v.GetBool("produced_surfaces", "windows") || v.GetBool("produced_surfaces", "linux") {
		t.Fatalf("unselected surfaces recorded true: %s", data)
	}
	refs := v.GetArray("manifest_refs")
	if len(refs) != 2 {
		t.Fatalf("manifest_refs len = %d, want 2", len(refs))
	}

	// Require the pack entries and no world or native artifacts.
	if got := string(v.GetStringBytes("pack", "path")); got != "manifest.pack.kvf" {
		t.Fatalf("pack path = %q", got)
	}
	if got := v.GetInt64("pack", "size"); got != 4 {
		t.Fatalf("pack size = %d, want 4", got)
	}
	if got := string(v.GetStringBytes("pack_metadata", "path")); got != "manifest-pack.bin" {
		t.Fatalf("pack metadata path = %q", got)
	}
	if v.Exists("artifacts") || v.Exists("world") {
		t.Fatalf("handoff manifest keeps the removed world and artifacts: %s", data)
	}
}

func TestWritePluginHandoffManifestRejectsNonArrayManifestRefs(t *testing.T) {
	// Stage the pack files and a manifest refs file that is not an array.
	root := t.TempDir()
	manifestRefsPath := filepath.Join(root, "manifest-refs.json")
	writePluginTestFile(t, filepath.Join(root, "manifest.pack.kvf"), []byte("pack"))
	writePluginTestFile(t, filepath.Join(root, "manifest-pack.bin"), []byte("metadata"))
	writePluginTestFile(t, manifestRefsPath, []byte(`{"manifest_id":"devtool"}`))

	// Require the handoff manifest to reject the refs.
	err := writePluginHandoffManifest(pluginHandoffOptions{
		rootDir:            root,
		manifestRefsPath:   manifestRefsPath,
		pluginRev:          "abc123",
		releaseEnvironment: "plugin-production",
		requestedSelection: "browser",
		gitSHA:             "abc123",
		runID:              "456",
		runAttempt:         "3",
		sourceRepo:         "s4wave/spacewave",
		workflow:           "plugin-release",
		includeBrowser:     true,
	})
	if err == nil || err.Error() != "manifest refs must be a JSON array" {
		t.Fatalf("expected manifest refs array rejection, got %v", err)
	}
}

func TestMarshalManifestInventoryStable(t *testing.T) {
	// Render two entries.
	got := marshalManifestInventory([]manifestInventoryEntry{
		{manifestID: "devtool", platformID: "browser/js", rev: 31, ref: "browser-ref"},
		{manifestID: "devtool", platformID: "darwin/arm64", rev: 31, ref: "darwin-ref"},
	})

	// Require the stable one-object-per-line form.
	want := `[
  {"manifest_id":"devtool","platform_id":"browser/js","rev":31,"ref":"browser-ref"},
  {"manifest_id":"devtool","platform_id":"darwin/arm64","rev":31,"ref":"darwin-ref"}
]
`
	if got != want {
		t.Fatalf("marshalManifestInventory() = %q, want %q", got, want)
	}
}

func writePluginTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
