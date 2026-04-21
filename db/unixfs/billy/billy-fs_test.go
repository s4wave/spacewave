package unixfs_billy_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

// newTestBillyFS creates a BillyFS backed by memfs for testing.
func newTestBillyFS(t *testing.T) (*unixfs_billy.BillyFS, context.Context) {
	t.Helper()
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err)
	}
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	t.Cleanup(fsc.Release)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsh.Release)
	ctx := context.Background()
	ts := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	return unixfs_billy.NewBillyFS(ctx, fsh, "", ts), ctx
}

// assertPathError checks that err is a *os.PathError with the expected op and path,
// and that os.IsNotExist returns true.
func assertPathError(t *testing.T, err error, op, filepath string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error for %s(%q), got nil", op, filepath)
	}
	var pe *os.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *os.PathError for %s(%q), got %T: %v", op, filepath, err, err)
	}
	if pe.Op != op {
		t.Errorf("PathError.Op = %q, want %q", pe.Op, op)
	}
	if pe.Path != filepath {
		t.Errorf("PathError.Path = %q, want %q", pe.Path, filepath)
	}
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist(%v) = false, want true", err)
	}
}

func TestBillyFS_ErrorWrapping(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	t.Run("Open", func(t *testing.T) {
		_, err := billyFS.Open("nonexistent")
		assertPathError(t, err, "open", "nonexistent")
	})

	t.Run("Stat", func(t *testing.T) {
		_, err := billyFS.Stat("nonexistent")
		assertPathError(t, err, "stat", "nonexistent")
	})

	t.Run("Lstat", func(t *testing.T) {
		_, err := billyFS.Lstat("nonexistent")
		assertPathError(t, err, "lstat", "nonexistent")
	})

	t.Run("Remove", func(t *testing.T) {
		err := billyFS.Remove("nonexistent")
		assertPathError(t, err, "remove", "nonexistent")
	})

	t.Run("ReadDir", func(t *testing.T) {
		_, err := billyFS.ReadDir("nonexistent")
		assertPathError(t, err, "readdir", "nonexistent")
	})

	t.Run("Readlink", func(t *testing.T) {
		_, err := billyFS.Readlink("nonexistent")
		assertPathError(t, err, "readlink", "nonexistent")
	})

	t.Run("Chroot", func(t *testing.T) {
		_, err := billyFS.Chroot("nonexistent")
		assertPathError(t, err, "chroot", "nonexistent")
	})

	t.Run("OpenFile", func(t *testing.T) {
		_, err := billyFS.OpenFile("nonexistent", os.O_RDONLY, 0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var pe *os.PathError
		if !errors.As(err, &pe) {
			t.Fatalf("expected *os.PathError, got %T: %v", err, err)
		}
		if !os.IsNotExist(err) {
			t.Errorf("os.IsNotExist(%v) = false, want true", err)
		}
	})
}

func TestBillyFS_SymlinkParentDirCreation(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	err := billyFS.Symlink("../target", "a/b/c/link")
	if err != nil {
		t.Fatalf("Symlink with nested parent dirs: %v", err)
	}

	target, err := billyFS.Readlink("a/b/c/link")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "../target" {
		t.Errorf("Readlink = %q, want %q", target, "../target")
	}
}

func TestBillyFS_SymlinkRelativeTargets(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	err := billyFS.Symlink("../../other/file", "link")
	if err != nil {
		t.Fatalf("Symlink with relative target: %v", err)
	}

	target, err := billyFS.Readlink("link")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "../../other/file" {
		t.Errorf("Readlink = %q, want %q", target, "../../other/file")
	}
}

func TestBillyFS_FileRoundtrip(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	content := []byte("hello world")

	f, err := billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err = billyFS.Open("testfile")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	fi, err := billyFS.Stat("testfile")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size() != int64(len(content)) {
		t.Errorf("Size = %d, want %d", fi.Size(), len(content))
	}

	if err := billyFS.Remove("testfile"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = billyFS.Stat("testfile")
	if !os.IsNotExist(err) {
		t.Errorf("Stat after remove: expected os.IsNotExist, got %v", err)
	}
}

func TestBillyFS_ReadDir(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	if err := billyFS.MkdirAll("subdir", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"subdir/a.txt", "subdir/b.txt"} {
		f, err := billyFS.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		f.Close()
	}

	entries, err := billyFS.ReadDir("subdir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir entries = %d, want 2", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("ReadDir entries = %v, want a.txt and b.txt", names)
	}

	t.Run("root", func(t *testing.T) {
		entries, err := billyFS.ReadDir(".")
		if err != nil {
			t.Fatalf("ReadDir root: %v", err)
		}
		found := false
		for _, e := range entries {
			if e.Name() == "subdir" {
				found = true
			}
		}
		if !found {
			t.Error("ReadDir root: subdir not found")
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		_, err := billyFS.ReadDir("nonexistent")
		if !os.IsNotExist(err) {
			t.Errorf("ReadDir nonexistent: expected os.IsNotExist, got %v", err)
		}
	})
}

func TestBillyFS_Chroot(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	if err := billyFS.MkdirAll("sub/dir", 0o755); err != nil {
		t.Fatal(err)
	}

	chrooted, err := billyFS.Chroot("sub")
	if err != nil {
		t.Fatalf("Chroot: %v", err)
	}
	if chrooted.Root() != "/sub" {
		t.Errorf("Root() = %q, want %q", chrooted.Root(), "/sub")
	}

	f, err := chrooted.Create("file.txt")
	if err != nil {
		t.Fatalf("Create in chroot: %v", err)
	}
	f.Write([]byte("chrooted"))
	f.Close()

	fi, err := chrooted.Stat("file.txt")
	if err != nil {
		t.Fatalf("Stat in chroot: %v", err)
	}
	if fi.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", fi.Name(), "file.txt")
	}
}

func TestBillyFS_OpenFileExclusive(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	f, err := billyFS.OpenFile("excl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile O_EXCL new: %v", err)
	}
	f.Close()

	_, err = billyFS.OpenFile("excl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		t.Fatal("expected error for O_EXCL on existing file")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("expected fs.ErrExist, got %v", err)
	}

	t.Run("rdonly_nonexistent", func(t *testing.T) {
		_, err := billyFS.OpenFile("nope", os.O_RDONLY, 0)
		if err == nil {
			t.Fatal("expected error")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected os.IsNotExist, got %v", err)
		}
		var pe *os.PathError
		if !errors.As(err, &pe) {
			t.Errorf("expected *os.PathError, got %T", err)
		}
	})
}

func TestBillyFS_LstatSymlink(t *testing.T) {
	billyFS, _ := newTestBillyFS(t)

	// Create a regular file as the symlink target.
	f, err := billyFS.OpenFile("target.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	f.Write([]byte("target content"))
	f.Close()

	// Create a symlink pointing to the target.
	if err := billyFS.Symlink("target.txt", "link.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Lstat does NOT follow the symlink: should return ModeSymlink.
	lstatInfo, err := billyFS.Lstat("link.txt")
	if err != nil {
		t.Fatalf("Lstat symlink: %v", err)
	}
	if lstatInfo.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Lstat mode = %v, want ModeSymlink set", lstatInfo.Mode())
	}

	// Lstat on the target itself should return a regular file.
	lstatTarget, err := billyFS.Lstat("target.txt")
	if err != nil {
		t.Fatalf("Lstat target: %v", err)
	}
	if !lstatTarget.Mode().IsRegular() {
		t.Errorf("Lstat target mode = %v, want regular file", lstatTarget.Mode())
	}

	// Readlink round-trip: target should match what was passed to Symlink.
	target, err := billyFS.Readlink("link.txt")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("Readlink = %q, want %q", target, "target.txt")
	}

	// Size must equal len(target) for go-git hash compatibility.
	if lstatInfo.Size() != int64(len(target)) {
		t.Errorf("Lstat Size = %d, want %d (len of readlink target)", lstatInfo.Size(), len(target))
	}

	// Symlink in a subdirectory with relative target.
	if err := billyFS.MkdirAll("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := billyFS.Symlink("../target.txt", "sub/nested-link"); err != nil {
		t.Fatalf("Symlink nested: %v", err)
	}

	lstatNested, err := billyFS.Lstat("sub/nested-link")
	if err != nil {
		t.Fatalf("Lstat nested symlink: %v", err)
	}
	if lstatNested.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Lstat nested mode = %v, want ModeSymlink set", lstatNested.Mode())
	}

	nestedTarget, err := billyFS.Readlink("sub/nested-link")
	if err != nil {
		t.Fatalf("Readlink nested: %v", err)
	}
	if nestedTarget != "../target.txt" {
		t.Errorf("Readlink nested = %q, want %q", nestedTarget, "../target.txt")
	}

	// Verify a directory Lstat returns ModeDir, not ModeSymlink.
	lstatDir, err := billyFS.Lstat("sub")
	if err != nil {
		t.Fatalf("Lstat dir: %v", err)
	}
	if !lstatDir.IsDir() {
		t.Errorf("Lstat dir mode = %v, want ModeDir set", lstatDir.Mode())
	}
}

// TestBillyFS_GitStatusSymlink tests that go-git's worktree.Status() reports
// a clean status after checking out a commit that contains symlinks into a
// BillyFS-backed worktree. This reproduces the production bug where all
// symlinks showed as Modified after checkout.
func TestBillyFS_GitStatusSymlink(t *testing.T) {
	// Create a BillyFS backed by memfs for the worktree.
	wtBfs := memfs.New()
	if err := wtBfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err)
	}
	fsc := unixfs_billy.NewBillyFSCursor(wtBfs, "")
	t.Cleanup(fsc.Release)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsh.Release)
	ctx := context.Background()
	ts := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	billyFS := unixfs_billy.NewBillyFS(ctx, fsh, "", ts)

	// Create an in-memory git storage and init a repo with BillyFS as worktree.
	gitStore := memory.NewStorage()
	repo, err := git.Init(
		gitStore,
		git.WithWorkTree(billyFS),
	)
	if err != nil {
		t.Fatalf("git.Init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	// Create a regular file and a symlink in the worktree.
	f, err := billyFS.OpenFile("hello.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("create hello.txt: %v", err)
	}
	f.Write([]byte("hello world\n"))
	f.Close()

	if err := billyFS.Symlink("hello.txt", "link.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if err := billyFS.Symlink("../../some/relative/path", "deep-link"); err != nil {
		t.Fatalf("Symlink deep: %v", err)
	}

	// Stage and commit everything.
	if _, err := wt.Add("hello.txt"); err != nil {
		t.Fatalf("Add hello.txt: %v", err)
	}
	if _, err := wt.Add("link.txt"); err != nil {
		t.Fatalf("Add link.txt: %v", err)
	}
	if _, err := wt.Add("deep-link"); err != nil {
		t.Fatalf("Add deep-link: %v", err)
	}

	commitHash, err := wt.Commit("initial commit with symlinks", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	t.Logf("committed: %s", commitHash)

	// Check status immediately after commit: should be clean.
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	for path, fs := range status {
		if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
			t.Errorf("file %q not clean: staging=%c worktree=%c", path, fs.Staging, fs.Worktree)
		}
	}

	if !status.IsClean() {
		t.Errorf("status not clean after commit:\n%s", status.String())
	}

	// Now simulate what happens in production: open the repo again with
	// a fresh BillyFS pointing to the same underlying storage, and check
	// status. This tests the Lstat/Readlink round-trip.
	fsc2 := unixfs_billy.NewBillyFSCursor(wtBfs, "")
	t.Cleanup(fsc2.Release)
	fsh2, err := unixfs.NewFSHandle(fsc2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsh2.Release)
	billyFS2 := unixfs_billy.NewBillyFS(ctx, fsh2, "", ts)

	repo2, err := git.Open(gitStore, billyFS2)
	if err != nil {
		t.Fatalf("git.Open: %v", err)
	}

	wt2, err := repo2.Worktree()
	if err != nil {
		t.Fatalf("Worktree2: %v", err)
	}

	status2, err := wt2.Status()
	if err != nil {
		t.Fatalf("Status2: %v", err)
	}

	for path, fs := range status2 {
		if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
			t.Errorf("re-opened: file %q not clean: staging=%c worktree=%c", path, fs.Staging, fs.Worktree)
		}
	}

	if !status2.IsClean() {
		t.Errorf("status not clean after re-open:\n%s", status2.String())
	}
}
