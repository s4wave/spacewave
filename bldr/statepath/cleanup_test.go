package statepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearBuildStatePartition(t *testing.T) {
	t.Run("cache on", func(t *testing.T) {
		root := newPartitionFixture(t)
		if err := ClearBuildState(root, true); err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{"logs", "src", "plugin", "cli"} {
			assertStatePathMissing(t, root, name)
		}
		for _, name := range []string{
			"build",
			"devtool.db",
			"devtool.db-shm",
			"devtool.s4wave",
			"devtool.s4wave-lock",
			"state-root.lock",
			"artifacts",
		} {
			assertStatePathExists(t, root, name)
		}
	})

	t.Run("cache off", func(t *testing.T) {
		root := newPartitionFixture(t)
		if err := ClearBuildState(root, false); err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{
			"logs",
			"src",
			"plugin",
			"cli",
			"build",
			"devtool.db",
			"devtool.db-shm",
			"devtool.s4wave",
			"devtool.s4wave-lock",
		} {
			assertStatePathMissing(t, root, name)
		}
		for _, name := range []string{"state-root.lock", "artifacts"} {
			assertStatePathExists(t, root, name)
		}
	})
}

func newPartitionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"logs", "src", "plugin", "cli", "build", "artifacts"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{
		"devtool.db",
		"devtool.db-shm",
		"devtool.s4wave",
		"devtool.s4wave-lock",
		"state-root.lock",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func assertStatePathExists(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, name)); err != nil {
		t.Fatalf("expected %s to exist: %v", name, err)
	}
}

func assertStatePathMissing(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed: %v", name, err)
	}
}
