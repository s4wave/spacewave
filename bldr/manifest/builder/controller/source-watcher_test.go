//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// TestSourceWatcher checks that changes made before Wait are kept, that touch
// counts as a change, that unwatched files and bare permission changes do not,
// and that a new file changes the DirSuffix input of its directory only when
// its name matches.
func TestSourceWatcher(t *testing.T) {
	// Bound the test so a missed event fails instead of hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Write an input file and a sibling in a temporary directory.
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.ts")
	otherPath := filepath.Join(dir, "other.ts")
	for _, p := range []string{inputPath, otherPath} {
		if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Watch the input file.
	inputFile := &bldr_manifest_builder.InputManifest_File{Path: "input.ts"}
	w, err := NewSourceWatcher(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]*bldr_manifest_builder.InputManifest_File{inputPath: inputFile}
	if err := w.SetPaths(paths); err != nil {
		t.Fatal(err)
	}

	// waitChanged waits for the next change and checks the changed file
	// is want.
	waitChanged := func(step string, want *bldr_manifest_builder.InputManifest_File) {
		t.Helper()
		files, events, err := w.Wait(ctx, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if len(files) != 1 || files[0] != want || events == 0 {
			t.Fatalf("%s: expected %v, got %v after %d events", step, want, files, events)
		}
	}

	// waitQuiet checks that no watched input changes for a while.
	waitQuiet := func(step string) {
		// Wait past the quiet window with nothing to take.
		t.Helper()
		quietCtx, quietCancel := context.WithTimeout(ctx, 300*time.Millisecond)
		files, _, err := w.Wait(quietCtx, 50*time.Millisecond)
		quietCancel()
		if err != context.DeadlineExceeded {
			t.Fatalf("%s: expected no change, got %v and %v", step, files, err)
		}
	}

	// A write made while no one waits, as during a build, is kept.
	if err := os.WriteFile(inputPath, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitChanged("write before wait", inputFile)

	// A write to an unwatched file is dropped, and a bare permission change
	// leaves the modification time alone, so neither wakes the waiter.
	if err := os.WriteFile(otherPath, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(inputPath, 0o600); err != nil {
		t.Fatal(err)
	}
	waitQuiet("unwatched write and chmod")

	// Touching the input moves its modification time and counts as a
	// change.
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(inputPath, now, now); err != nil {
		t.Fatal(err)
	}
	waitChanged("touch", inputFile)

	// Watch a schema directory as a DirSuffix input.
	schemaDir := filepath.Join(dir, "migrations")
	if err := os.Mkdir(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	schemaInput := &bldr_manifest_builder.InputManifest_File{Path: "migrations", DirSuffix: ".sql"}
	paths[schemaDir] = schemaInput
	if err := w.SetPaths(paths); err != nil {
		t.Fatal(err)
	}

	// Creating the directory changed it. A file without the suffix does
	// not, and a schema file does.
	waitChanged("schema dir created", schemaInput)
	if err := os.WriteFile(filepath.Join(schemaDir, "notes.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitQuiet("unmatched file in schema dir")
	if err := os.WriteFile(filepath.Join(schemaDir, "0002_more.sql"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitChanged("new schema file", schemaInput)
}
