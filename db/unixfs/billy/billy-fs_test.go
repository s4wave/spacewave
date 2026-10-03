package unixfs_billy_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/iotest"
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
	// Create an in-memory Billy root for the test filesystem.
	t.Helper()
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err)
	}

	// Retain a Billy cursor for the test filesystem lifetime.
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	t.Cleanup(fsc.Release)

	// Retain a UnixFS handle for the test filesystem lifetime.
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsh.Release)

	// Construct the Billy adapter with a fixed write timestamp.
	ctx := context.Background()
	ts := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	return unixfs_billy.NewBillyFS(ctx, fsh, "", ts), ctx
}

// assertPathError checks that err is a *os.PathError with the expected op and path,
// and that os.IsNotExist returns true.
func assertPathError(t *testing.T, err error, op, filepath string) {
	// Verify the missing-entry error identifies its Billy operation and path.
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
	// Create the Billy filesystem used by the missing-entry checks.
	billyFS, _ := newTestBillyFS(t)

	// Check the error returned when opening a missing Billy file.
	t.Run("Open", func(t *testing.T) {
		// Attempt to open the missing Billy file.
		_, err := billyFS.Open("nonexistent")

		// Verify the Billy open error identifies the missing path.
		assertPathError(t, err, "open", "nonexistent")
	})

	// Check the metadata error for a missing Billy file.
	t.Run("Stat", func(t *testing.T) {
		// Request metadata for the missing Billy file.
		_, err := billyFS.Stat("nonexistent")

		// Verify the Billy stat error identifies the missing path.
		assertPathError(t, err, "stat", "nonexistent")
	})

	// Check the error returned by Lstat for a missing Billy file.
	t.Run("Lstat", func(t *testing.T) {
		// Request link metadata for the missing Billy file.
		_, err := billyFS.Lstat("nonexistent")

		// Verify the Billy Lstat error identifies the missing path.
		assertPathError(t, err, "lstat", "nonexistent")
	})

	// Check the error returned when removing a missing Billy file.
	t.Run("Remove", func(t *testing.T) {
		// Attempt to remove the missing Billy file.
		err := billyFS.Remove("nonexistent")

		// Verify the Billy remove error identifies the missing path.
		assertPathError(t, err, "remove", "nonexistent")
	})

	// Check the error returned when listing a missing Billy directory.
	t.Run("ReadDir", func(t *testing.T) {
		// Attempt to list the missing Billy directory.
		_, err := billyFS.ReadDir("nonexistent")

		// Verify the Billy listing error identifies the missing path.
		assertPathError(t, err, "readdir", "nonexistent")
	})

	// Check the error returned when reading a missing Billy symbolic link.
	t.Run("Readlink", func(t *testing.T) {
		// Attempt to read the missing Billy symbolic link.
		_, err := billyFS.Readlink("nonexistent")

		// Verify the Billy Readlink error identifies the missing path.
		assertPathError(t, err, "readlink", "nonexistent")
	})

	// Check the error returned when changing to a missing Billy root.
	t.Run("Chroot", func(t *testing.T) {
		// Attempt to change the Billy root to a missing directory.
		_, err := billyFS.Chroot("nonexistent")

		// Verify the Billy Chroot error identifies the missing path.
		assertPathError(t, err, "chroot", "nonexistent")
	})

	// Check the OpenFile error for a missing Billy file.
	t.Run("OpenFile", func(t *testing.T) {
		// Attempt to open a missing Billy file with read-only flags.
		_, err := billyFS.OpenFile("nonexistent", os.O_RDONLY, 0)

		// Verify the Billy OpenFile error preserves the missing-path contract.
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := errors.AsType[*os.PathError](err); !ok {
			t.Fatalf("expected *os.PathError, got %T: %v", err, err)
		}
		if !os.IsNotExist(err) {
			t.Errorf("os.IsNotExist(%v) = false, want true", err)
		}
	})
}

func TestBillyFS_SymlinkParentDirCreation(t *testing.T) {
	// Create the Billy filesystem for nested symbolic link creation.
	billyFS, _ := newTestBillyFS(t)

	// Create a Billy symbolic link with missing parent directories.
	err := billyFS.Symlink("../target", "a/b/c/link")
	if err != nil {
		t.Fatalf("Symlink with nested parent dirs: %v", err)
	}

	// Read the nested Billy symbolic link target.
	target, err := billyFS.Readlink("a/b/c/link")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}

	// Verify the nested Billy symbolic link preserves its relative target.
	if target != "../target" {
		t.Errorf("Readlink = %q, want %q", target, "../target")
	}
}

func TestBillyFS_SymlinkRelativeTargets(t *testing.T) {
	// Create the Billy filesystem for relative symbolic link targets.
	billyFS, _ := newTestBillyFS(t)

	// Create a Billy symbolic link whose target traverses parent directories.
	err := billyFS.Symlink("../../other/file", "link")
	if err != nil {
		t.Fatalf("Symlink with relative target: %v", err)
	}

	// Read the relative Billy symbolic link target.
	target, err := billyFS.Readlink("link")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}

	// Verify the Billy symbolic link preserves the parent traversal.
	if target != "../../other/file" {
		t.Errorf("Readlink = %q, want %q", target, "../../other/file")
	}
}

func TestBillyFS_FileRoundtrip(t *testing.T) {
	// Create the Billy filesystem for the file content round trip.
	billyFS, _ := newTestBillyFS(t)

	// Prepare the content for the Billy file round trip.
	content := []byte("hello world")

	// Open a new Billy file for the round trip.
	f, err := billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile create: %v", err)
	}

	// Write and close the Billy file before reopening it.
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen the Billy file for reading.
	f, err = billyFS.Open("testfile")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Read and close the Billy file after the round trip.
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify the Billy file retains the written content.
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	// Read the Billy file metadata after writing its content.
	fi, err := billyFS.Stat("testfile")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	// Verify the Billy file size matches the written content.
	if fi.Size() != int64(len(content)) {
		t.Errorf("Size = %d, want %d", fi.Size(), len(content))
	}

	// Remove the Billy file after the round trip.
	if err := billyFS.Remove("testfile"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify the removed Billy file is absent.
	_, err = billyFS.Stat("testfile")
	if !os.IsNotExist(err) {
		t.Errorf("Stat after remove: expected os.IsNotExist, got %v", err)
	}
}

// TestBillyFS_ReadFromShortReads copies a reader that returns one byte per
// Read, which ReadFrom gathers into full writes.
func TestBillyFS_ReadFromShortReads(t *testing.T) {
	// Copy the content into a new file.
	billyFS, _ := newTestBillyFS(t)
	content := strings.Repeat("0123456789", 300)
	f, err := billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(f, iotest.OneByteReader(strings.NewReader(content)))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Check the count and the file content.
	if n != int64(len(content)) {
		t.Fatalf("copied %d bytes, want %d", n, len(content))
	}
	f, err = billyFS.Open("testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content has %d bytes, want %d", len(got), len(content))
	}
}

func TestBillyFS_OpenFileTruncatesExisting(t *testing.T) {
	// Create the Billy filesystem for file truncation.
	billyFS, _ := newTestBillyFS(t)

	// Create a Billy file containing the original content.
	f, err := billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile create: %v", err)
	}
	if _, err := f.Write([]byte("long original content")); err != nil {
		t.Fatalf("Write original: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close original: %v", err)
	}

	// Replace the Billy file content through a truncating open.
	f, err = billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile truncate: %v", err)
	}
	if _, err := f.Write([]byte("short")); err != nil {
		t.Fatalf("Write replacement: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close replacement: %v", err)
	}

	// Reopen and read the truncated Billy file.
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

	// Verify the Billy file contains only the replacement content.
	if string(got) != "short" {
		t.Fatalf("content = %q, want %q", got, "short")
	}
}

func TestBillyFS_OpenFileTruncateRequiresWriteAccess(t *testing.T) {
	// Create the Billy filesystem for the truncate access check.
	billyFS, _ := newTestBillyFS(t)

	// Create the Billy file whose content must survive the rejected open.
	f, err := billyFS.OpenFile("testfile", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile create: %v", err)
	}
	if _, err := f.Write([]byte("preserve")); err != nil {
		t.Fatalf("Write original: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close original: %v", err)
	}

	// Verify a truncating Billy open requires write access.
	f, err = billyFS.OpenFile("testfile", os.O_TRUNC, 0o644)
	if err == nil {
		_ = f.Close()
		t.Fatal("OpenFile O_TRUNC without write access succeeded")
	}

	// Reopen and read the Billy file after the rejected truncation.
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

	// Verify the rejected Billy open preserved the file content.
	if string(got) != "preserve" {
		t.Fatalf("content = %q, want %q", got, "preserve")
	}
}

func TestBillyFS_ReadDir(t *testing.T) {
	// Create the Billy filesystem for directory listing.
	billyFS, _ := newTestBillyFS(t)

	// Create a Billy directory containing two files.
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

	// Read the Billy directory entries.
	entries, err := billyFS.ReadDir("subdir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	// Verify the Billy listing contains two entries.
	if len(entries) != 2 {
		t.Fatalf("ReadDir entries = %d, want 2", len(entries))
	}

	// Verify the Billy listing contains both created filenames.
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("ReadDir entries = %v, want a.txt and b.txt", names)
	}

	// Check the Billy root directory listing.
	t.Run("root", func(t *testing.T) {
		// Read the Billy root directory entries.
		entries, err := billyFS.ReadDir(".")
		if err != nil {
			t.Fatalf("ReadDir root: %v", err)
		}

		// Verify the Billy root listing contains the created directory.
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

	// Check the error for a missing Billy directory listing.
	t.Run("nonexistent", func(t *testing.T) {
		// Attempt to list the missing Billy directory.
		_, err := billyFS.ReadDir("nonexistent")

		// Verify the Billy listing reports a missing directory.
		if !os.IsNotExist(err) {
			t.Errorf("ReadDir nonexistent: expected os.IsNotExist, got %v", err)
		}
	})
}

func TestBillyFS_Chroot(t *testing.T) {
	// Create the Billy filesystem for changing its root.
	billyFS, _ := newTestBillyFS(t)

	// Create the Billy directory tree used by Chroot.
	if err := billyFS.MkdirAll("sub/dir", 0o755); err != nil {
		t.Fatal(err)
	}

	// Change the Billy filesystem root to the created directory.
	chrooted, err := billyFS.Chroot("sub")
	if err != nil {
		t.Fatalf("Chroot: %v", err)
	}

	// Verify the Billy filesystem reports its new root path.
	if chrooted.Root() != "/sub" {
		t.Errorf("Root() = %q, want %q", chrooted.Root(), "/sub")
	}

	// Write a Billy file through the changed root.
	f, err := chrooted.Create("file.txt")
	if err != nil {
		t.Fatalf("Create in chroot: %v", err)
	}
	f.Write([]byte("chrooted"))
	f.Close()

	// Read metadata for the Billy file through the changed root.
	fi, err := chrooted.Stat("file.txt")
	if err != nil {
		t.Fatalf("Stat in chroot: %v", err)
	}

	// Verify the Billy file retains its name under the changed root.
	if fi.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", fi.Name(), "file.txt")
	}
}

func TestBillyFS_OpenFileExclusive(t *testing.T) {
	// Create the Billy filesystem for exclusive file creation.
	billyFS, _ := newTestBillyFS(t)

	// Create the initial Billy file with exclusive flags.
	f, err := billyFS.OpenFile("excl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile O_EXCL new: %v", err)
	}
	f.Close()

	// Verify exclusive Billy creation rejects the existing file.
	_, err = billyFS.OpenFile("excl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		t.Fatal("expected error for O_EXCL on existing file")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("expected fs.ErrExist, got %v", err)
	}

	// Check a read-only Billy open of a missing file.
	t.Run("rdonly_nonexistent", func(t *testing.T) {
		// Attempt to open the missing Billy file read-only.
		_, err := billyFS.OpenFile("nope", os.O_RDONLY, 0)

		// Verify the Billy open returns a missing-path error.
		if err == nil {
			t.Fatal("expected error")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected os.IsNotExist, got %v", err)
		}
		if _, ok := errors.AsType[*os.PathError](err); !ok {
			t.Errorf("expected *os.PathError, got %T", err)
		}
	})
}

func TestBillyFS_LstatSymlink(t *testing.T) {
	// Create the Billy filesystem for symbolic link metadata checks.
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

	// Read metadata for the nested Billy symbolic link.
	lstatNested, err := billyFS.Lstat("sub/nested-link")
	if err != nil {
		t.Fatalf("Lstat nested symlink: %v", err)
	}
	if lstatNested.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Lstat nested mode = %v, want ModeSymlink set", lstatNested.Mode())
	}

	// Read the nested Billy symbolic link target.
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

	// Retain a Billy cursor for the Git worktree filesystem.
	fsc := unixfs_billy.NewBillyFSCursor(wtBfs, "")
	t.Cleanup(fsc.Release)

	// Retain a UnixFS handle for the Git worktree filesystem.
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsh.Release)

	// Construct the Billy adapter with a fixed Git worktree timestamp.
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

	// Retrieve the Git worktree backed by the Billy filesystem.
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

	// Create a Billy symbolic link to the worktree file.
	if err := billyFS.Symlink("hello.txt", "link.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Create a Billy symbolic link with a parent-relative target.
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

	// Commit the Git worktree entries containing symbolic links.
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

	// Verify every Git entry is unchanged after the commit.
	for path, fs := range status {
		if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
			t.Errorf("file %q not clean: staging=%c worktree=%c", path, fs.Staging, fs.Worktree)
		}
	}

	// Verify the Git worktree is clean after the commit.
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

	// Reopen the Git repository through the fresh Billy filesystem.
	repo2, err := git.Open(gitStore, billyFS2)
	if err != nil {
		t.Fatalf("git.Open: %v", err)
	}

	// Retrieve the reopened Git worktree.
	wt2, err := repo2.Worktree()
	if err != nil {
		t.Fatalf("Worktree2: %v", err)
	}

	// Read the reopened Git worktree status.
	status2, err := wt2.Status()
	if err != nil {
		t.Fatalf("Status2: %v", err)
	}

	// Verify every Git entry is unchanged after reopening.
	for path, fs := range status2 {
		if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
			t.Errorf("re-opened: file %q not clean: staging=%c worktree=%c", path, fs.Staging, fs.Worktree)
		}
	}

	// Verify the reopened Git worktree is clean.
	if !status2.IsClean() {
		t.Errorf("status not clean after re-open:\n%s", status2.String())
	}
}
