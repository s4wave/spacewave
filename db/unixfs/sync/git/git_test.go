package unixfs_sync_git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestSyncFromGitWorkdir copies tracked and untracked files while excluding ignored files.
func TestSyncFromGitWorkdir(t *testing.T) {
	// Populate a Git fixture independently of the source checkout's metadata.
	srcRoot := t.TempDir()
	files := map[string]string{
		".gitignore":    "ignored.txt\n",
		"tracked.txt":   "tracked content",
		"untracked.txt": "untracked content",
		"ignored.txt":   "ignored content",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(srcRoot, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Stage one file, leaving the others to Git's untracked and ignored rules.
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "tracked.txt"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = srcRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	// Start a World-backed filesystem under the test's lifecycle.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err)
	}

	// Attach the filesystem handle and synchronize the Git fixture into it.
	fsHandle, err := unixfs_world_testbed.InitTestbed(wtb, "test/fs", true)
	if err != nil {
		t.Fatal(err)
	}
	defer fsHandle.Release()
	if err := SyncFromGitWorkdir(ctx, fsHandle, srcRoot, unixfs_sync.DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}

	// Read every included file back through UnixFS.
	for _, name := range []string{".gitignore", "tracked.txt", "untracked.txt"} {
		h, err := fsHandle.Lookup(ctx, name)
		if err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}
		data, err := unixfs.ReadFile(ctx, h)
		h.Release()
		if err != nil || string(data) != files[name] {
			t.Fatalf("read %s: %q, %v; want %q", name, data, err, files[name])
		}
	}

	// Require exactly the three Git-selected files at the destination root.
	var entries int
	if err := fsHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		if ent.GetIsFile() {
			entries++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if entries != 3 {
		t.Fatalf("synchronized %d root files, want 3", entries)
	}
}
