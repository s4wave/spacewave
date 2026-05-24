package localdist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths(t *testing.T) {
	exePath := "/Applications/Spacewave.app/Contents/MacOS/Spacewave"
	paths := Paths(exePath)
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

func TestRead(t *testing.T) {
	td := t.TempDir()
	want := []byte("signed-config")
	p := filepath.Join(td, Filename)
	if err := os.WriteFile(p, want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, gotPath, err := Read([]string{
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

func TestReadEmpty(t *testing.T) {
	td := t.TempDir()
	p := filepath.Join(td, Filename)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Read([]string{p}); err == nil {
		t.Fatal("expected empty local dist config error")
	}
}
