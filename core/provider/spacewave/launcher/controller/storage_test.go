package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalDistConfPaths(t *testing.T) {
	exePath := "/Applications/Spacewave.app/Contents/MacOS/Spacewave"
	paths := localDistConfPaths(exePath)
	if len(paths) != 2 {
		t.Fatalf("expected 2 candidate paths, got %d", len(paths))
	}
	if got, want := paths[0], "/Applications/Spacewave.app/Contents/MacOS/dist-config.packedmsg"; got != want {
		t.Fatalf("first path = %q, want %q", got, want)
	}
	if got, want := paths[1], "/Applications/Spacewave.app/Contents/Resources/dist-config.packedmsg"; got != want {
		t.Fatalf("second path = %q, want %q", got, want)
	}
}

func TestReadLocalDistConf(t *testing.T) {
	td := t.TempDir()
	want := []byte("signed-config")
	p := filepath.Join(td, localDistConfigFilename)
	if err := os.WriteFile(p, want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, gotPath, err := readLocalDistConf([]string{
		filepath.Join(td, "missing"),
		p,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != p {
		t.Fatalf("path = %q, want %q", gotPath, p)
	}
	if string(got) != string(want) {
		t.Fatalf("data = %q, want %q", string(got), string(want))
	}
}

func TestReadLocalDistConfEmpty(t *testing.T) {
	td := t.TempDir()
	p := filepath.Join(td, localDistConfigFilename)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := readLocalDistConf([]string{p}); err == nil {
		t.Fatal("expected empty local dist config error")
	}
}
