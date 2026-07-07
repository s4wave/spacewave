//go:build !js

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

func TestWritePluginHandoffManifestRecordsSurfacesAndArtifacts(t *testing.T) {
	root := t.TempDir()
	worldPath := filepath.Join(root, "devtool.s4wave")
	manifestRefsPath := filepath.Join(root, "manifest-refs.json")
	nativePath := filepath.Join(root, "native", "darwin", "arm64", "plugin.tar.gz")
	writePluginTestFile(t, worldPath, []byte("world"))
	writePluginTestFile(t, nativePath, []byte("artifact"))
	writePluginTestFile(t, manifestRefsPath, []byte(`[
  {"manifest_id":"devtool","platform_id":"browser/js","rev":31,"ref":"browser-ref"},
  {"manifest_id":"devtool","platform_id":"darwin/arm64","rev":31,"ref":"darwin-ref"}
]`))

	if err := writePluginHandoffManifest(pluginHandoffOptions{
		rootDir:            root,
		worldPath:          worldPath,
		manifestRefsPath:   manifestRefsPath,
		nativeDir:          filepath.Join(root, "native"),
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

	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
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
	if got := refs[0].GetUint("rev"); got != 31 {
		t.Fatalf("first manifest ref rev = %d, want 31", got)
	}
	if got := refs[1].GetUint("rev"); got != 31 {
		t.Fatalf("second manifest ref rev = %d, want 31", got)
	}
	if got := string(v.GetStringBytes("world", "path")); got != "devtool.s4wave" {
		t.Fatalf("world path = %q", got)
	}
	artifacts := v.GetArray("artifacts")
	if len(artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(artifacts))
	}
	if got := string(artifacts[0].GetStringBytes("path")); got != "darwin/arm64/plugin.tar.gz" {
		t.Fatalf("artifact path = %q", got)
	}
}

func TestWritePluginHandoffManifestRejectsNonArrayManifestRefs(t *testing.T) {
	root := t.TempDir()
	worldPath := filepath.Join(root, "devtool.s4wave")
	manifestRefsPath := filepath.Join(root, "manifest-refs.json")
	writePluginTestFile(t, worldPath, []byte("world"))
	writePluginTestFile(t, manifestRefsPath, []byte(`{"manifest_id":"devtool"}`))

	err := writePluginHandoffManifest(pluginHandoffOptions{
		rootDir:            root,
		worldPath:          worldPath,
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
func TestWritePluginHandoffManifestRejectsZeroManifestRefRev(t *testing.T) {
	root := t.TempDir()
	worldPath := filepath.Join(root, "devtool.s4wave")
	manifestRefsPath := filepath.Join(root, "manifest-refs.json")
	writePluginTestFile(t, worldPath, []byte("world"))
	writePluginTestFile(t, manifestRefsPath, []byte(`[
  {"manifest_id":"spacewave-notes","platform_id":"js","rev":0,"ref":"notes-ref"}
]`))

	err := writePluginHandoffManifest(pluginHandoffOptions{
		rootDir:            root,
		worldPath:          worldPath,
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
	if err == nil || !strings.Contains(err.Error(), "manifest ref rev must be non-zero: spacewave-notes/js") {
		t.Fatalf("expected manifest ref rev rejection, got %v", err)
	}
}

func TestMarshalManifestInventoryStable(t *testing.T) {
	got := marshalManifestInventory([]manifestInventoryEntry{
		{manifestID: "devtool", platformID: "browser/js", rev: 31, ref: "browser-ref"},
		{manifestID: "devtool", platformID: "darwin/arm64", rev: 31, ref: "darwin-ref"},
	})
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
