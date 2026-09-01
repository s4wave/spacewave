package bldr_dist_compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyWebAssetPackSplitsLogicalFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "output")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	parts, err := copyWebAssetPack(source, output, "../hash/", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	for i, want := range []string{"abcd", "efgh", "ij"} {
		got, err := os.ReadFile(filepath.Join(output, filepath.Base(parts[i].URL)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("part %d = %q, want %q", i, got, want)
		}
	}
}

func TestCopyWebAssetPackKeepsSingleFileLayout(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "output")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	parts, err := copyWebAssetPack(source, output, "../hash/", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].URL != "../hash/assets.kvfile" {
		t.Fatalf("parts = %#v, want legacy assets.kvfile", parts)
	}
}
