//go:build !js

package checkoutroot

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestUploadStageSweep checks that cleanup retains active and unowned files.
func TestUploadStageSweep(t *testing.T) {
	// Place a named payload and its locked owner marker in a nested directory.
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := root.Mkdir("nested", 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := root.OpenRoot("nested")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	stage, err := openNamedStage(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.discard() })
	if _, err := stage.file.WriteString("partial"); err != nil {
		t.Fatal(err)
	}

	// Keep an ordinary file with no lease beside the hidden active payload.
	unowned := uploadStagePrefix + strings.Repeat("A", 26)
	if err := dir.WriteFile(unowned, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hiddenUploadName(stage.name) || !hiddenUploadName(stage.name+uploadLeaseSuffix) {
		t.Fatal("staging entries must be hidden")
	}
	le := logrus.New().WithField("test", t.Name())
	sweepUploads(t.Context(), le, root)
	if _, err := dir.Stat(stage.name); err != nil {
		t.Fatalf("active upload was removed: %v", err)
	}

	// Drop the open handles as a killed publisher would and reclaim its stage.
	if err := errors.Join(stage.file.Close(), stage.lease.Close()); err != nil {
		t.Fatal(err)
	}
	sweepUploads(t.Context(), le, root)
	for _, name := range []string{stage.name, stage.name + uploadLeaseSuffix} {
		if _, err := dir.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("abandoned stage %s was retained: %v", name, err)
		}
	}
	if data, err := dir.ReadFile(unowned); err != nil || string(data) != "keep" {
		t.Fatalf("unowned file = %q, error %v", data, err)
	}
}
