package unixfs_git

import (
	"context"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
)

// storeBlob writes a blob to the storage and returns its hash.
func storeBlob(t *testing.T, s *memory.Storage, content string) plumbing.Hash {
	t.Helper()
	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// storeTree writes a tree to the storage and returns its hash.
func storeTree(t *testing.T, s *memory.Storage, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()
	sort.Sort(object.TreeEntrySorter(entries))
	tree := &object.Tree{Entries: entries}
	obj := s.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// buildTestTree creates a test tree with various entry types in memory storage.
// Returns the root tree object.
//
// Structure:
//
//	/
//	├── README.md          (regular file, "# Hello World\n")
//	├── build.sh           (executable file, "#!/bin/sh\necho hello\n")
//	├── docs/              (directory)
//	│   └── guide.txt      (regular file, "User guide content")
//	├── empty/             (empty directory)
//	├── link.txt           (symlink -> README.md)
//	└── src/               (directory)
//	    └── main.go        (regular file, "package main\n")
func buildTestTree(t *testing.T, s *memory.Storage) *object.Tree {
	t.Helper()

	readmeHash := storeBlob(t, s, "# Hello World\n")
	buildShHash := storeBlob(t, s, "#!/bin/sh\necho hello\n")
	guideHash := storeBlob(t, s, "User guide content")
	mainGoHash := storeBlob(t, s, "package main\n")
	linkHash := storeBlob(t, s, "README.md")

	// docs/ subtree
	docsTreeHash := storeTree(t, s, []object.TreeEntry{
		{Name: "guide.txt", Mode: filemode.Regular, Hash: guideHash},
	})

	// empty/ subtree (empty tree)
	emptyTreeHash := storeTree(t, s, nil)

	// src/ subtree
	srcTreeHash := storeTree(t, s, []object.TreeEntry{
		{Name: "main.go", Mode: filemode.Regular, Hash: mainGoHash},
	})

	// root tree
	rootHash := storeTree(t, s, []object.TreeEntry{
		{Name: "README.md", Mode: filemode.Regular, Hash: readmeHash},
		{Name: "build.sh", Mode: filemode.Executable, Hash: buildShHash},
		{Name: "docs", Mode: filemode.Dir, Hash: docsTreeHash},
		{Name: "empty", Mode: filemode.Dir, Hash: emptyTreeHash},
		{Name: "link.txt", Mode: filemode.Symlink, Hash: linkHash},
		{Name: "src", Mode: filemode.Dir, Hash: srcTreeHash},
	})

	tree, err := object.GetTree(s, rootHash)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestReaddirAll(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !ops.GetIsDirectory() {
		t.Fatal("expected root to be a directory")
	}
	if ops.GetName() != "" {
		t.Fatalf("expected empty name for root, got %q", ops.GetName())
	}

	var names []string
	err = ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"README.md", "build.sh", "docs", "empty", "link.txt", "src"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("entry %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestReaddirAllSkip(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	err = ops.ReaddirAll(ctx, 3, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"empty", "link.txt", "src"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("entry %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestReaddirAllNodeTypes(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	type entInfo struct {
		name   string
		isDir  bool
		isFile bool
		isLink bool
	}

	var entries []entInfo
	err = ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entries = append(entries, entInfo{
			name:   ent.GetName(),
			isDir:  ent.GetIsDirectory(),
			isFile: ent.GetIsFile(),
			isLink: ent.GetIsSymlink(),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []entInfo{
		{name: "README.md", isFile: true},
		{name: "build.sh", isFile: true},
		{name: "docs", isDir: true},
		{name: "empty", isDir: true},
		{name: "link.txt", isLink: true},
		{name: "src", isDir: true},
	}

	for i, e := range expected {
		if entries[i] != e {
			t.Fatalf("entry %d: expected %+v, got %+v", i, e, entries[i])
		}
	}
}

func TestLookupFile(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !childOps.GetIsFile() {
		t.Fatal("expected README.md to be a file")
	}
	if childOps.GetIsDirectory() {
		t.Fatal("expected README.md not to be a directory")
	}
	if childOps.GetName() != "README.md" {
		t.Fatalf("expected name 'README.md', got %q", childOps.GetName())
	}
}

func TestReadAtFile(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// read entire file
	buf := make([]byte, 100)
	n, err := childOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	content := string(buf[:n])
	if content != "# Hello World\n" {
		t.Fatalf("expected '# Hello World\\n', got %q", content)
	}
}

func TestReadAtOffset(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// read from offset 2 ("Hello World\n")
	buf := make([]byte, 100)
	n, err := childOps.ReadAt(ctx, 2, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	content := string(buf[:n])
	if content != "Hello World\n" {
		t.Fatalf("expected 'Hello World\\n', got %q", content)
	}
}

func TestReadAtPastEOF(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 10)
	n, err := childOps.ReadAt(ctx, 1000, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes, got %d", n)
	}
}

func TestFileSize(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	size, err := childOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 14 { // "# Hello World\n" = 14 bytes
		t.Fatalf("expected size 14, got %d", size)
	}
}

func TestDirectorySize(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	size, err := ops.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("expected directory size 0, got %d", size)
	}
}

func TestFilePermissions(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		perm string // expected permission string representation
	}{
		{"README.md", "-rw-r--r--"},
		{"build.sh", "-rwxr-xr-x"},
	}

	for _, tt := range tests {
		child, err := ops.Lookup(ctx, tt.name)
		if err != nil {
			t.Fatalf("lookup %s: %v", tt.name, err)
		}
		childOps, err := child.GetCursorOps(ctx)
		if err != nil {
			t.Fatalf("getops %s: %v", tt.name, err)
		}
		perm, err := childOps.GetPermissions(ctx)
		if err != nil {
			t.Fatalf("getperm %s: %v", tt.name, err)
		}
		if perm.String() != tt.perm {
			t.Fatalf("%s: expected permissions %s, got %s", tt.name, tt.perm, perm.String())
		}
		child.Release()
	}
}

func TestRootPermissions(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	perm, err := ops.GetPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// root dir: 0755 | ModeDir
	if perm.String() != "drwxr-xr-x" {
		t.Fatalf("expected drwxr-xr-x, got %s", perm.String())
	}
}

func TestLookupNotExist(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ops.Lookup(ctx, "nonexistent.txt")
	if err != unixfs_errors.ErrNotExist {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestSubdirectoryNavigation(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// navigate to docs/
	docsChild, err := ops.Lookup(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	defer docsChild.Release()

	docsOps, err := docsChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !docsOps.GetIsDirectory() {
		t.Fatal("expected docs to be a directory")
	}
	if docsOps.GetName() != "docs" {
		t.Fatalf("expected name 'docs', got %q", docsOps.GetName())
	}

	// navigate to docs/guide.txt
	guideChild, err := docsOps.Lookup(ctx, "guide.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer guideChild.Release()

	guideOps, err := guideChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !guideOps.GetIsFile() {
		t.Fatal("expected guide.txt to be a file")
	}

	buf := make([]byte, 100)
	n, err := guideOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "User guide content" {
		t.Fatalf("unexpected content: %q", string(buf[:n]))
	}
}

func TestEmptyDirectory(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "empty")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !childOps.GetIsDirectory() {
		t.Fatal("expected empty to be a directory")
	}

	var count int
	err = childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 entries in empty dir, got %d", count)
	}
}

func TestSymlinkLookup(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !childOps.GetIsSymlink() {
		t.Fatal("expected link.txt to be a symlink")
	}
	if childOps.GetIsFile() {
		t.Fatal("expected symlink not to be a file")
	}
	if childOps.GetIsDirectory() {
		t.Fatal("expected symlink not to be a directory")
	}
}

func TestReadlinkSelf(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	parts, isAbsolute, err := childOps.Readlink(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if isAbsolute {
		t.Fatal("expected relative link")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkFromDir(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	parts, isAbsolute, err := ops.Readlink(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if isAbsolute {
		t.Fatal("expected relative link")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkAbsolute(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()

	// create a symlink with absolute target
	linkHash := storeBlob(t, s, "/usr/local/bin/foo")
	rootHash := storeTree(t, s, []object.TreeEntry{
		{Name: "abs-link", Mode: filemode.Symlink, Hash: linkHash},
	})

	tree, err := object.GetTree(s, rootHash)
	if err != nil {
		t.Fatal(err)
	}

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	parts, isAbsolute, err := ops.Readlink(ctx, "abs-link")
	if err != nil {
		t.Fatal(err)
	}
	if !isAbsolute {
		t.Fatal("expected absolute link")
	}
	expected := []string{"usr", "local", "bin", "foo"}
	if len(parts) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, parts)
	}
	for i, p := range parts {
		if p != expected[i] {
			t.Fatalf("part %d: expected %q, got %q", i, expected[i], p)
		}
	}
}

func TestReadlinkNotSymlink(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ops.Readlink(ctx, "README.md")
	if err != unixfs_errors.ErrNotSymlink {
		t.Fatalf("expected ErrNotSymlink, got %v", err)
	}
}

func TestWriteOpsReturnReadOnly(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// SetPermissions
	if err := ops.SetPermissions(ctx, 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("SetPermissions: expected ErrReadOnly, got %v", err)
	}

	// SetModTimestamp
	if err := ops.SetModTimestamp(ctx, time.Now()); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("SetModTimestamp: expected ErrReadOnly, got %v", err)
	}

	// WriteAt
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("WriteAt: expected ErrReadOnly, got %v", err)
	}

	// GetOptimalWriteSize
	if _, err := ops.GetOptimalWriteSize(ctx); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("GetOptimalWriteSize: expected ErrReadOnly, got %v", err)
	}

	// Truncate
	if err := ops.Truncate(ctx, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Truncate: expected ErrReadOnly, got %v", err)
	}

	// Mknod
	if err := ops.Mknod(ctx, true, []string{"x"}, nil, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Mknod: expected ErrReadOnly, got %v", err)
	}

	// Symlink
	if err := ops.Symlink(ctx, true, "x", []string{"y"}, false, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Symlink: expected ErrReadOnly, got %v", err)
	}

	// Remove
	if err := ops.Remove(ctx, []string{"x"}, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Remove: expected ErrReadOnly, got %v", err)
	}

	// MoveTo
	if _, err := ops.MoveTo(ctx, nil, "x", time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("MoveTo: expected ErrReadOnly, got %v", err)
	}

	// MoveFrom
	if _, err := ops.MoveFrom(ctx, "x", nil, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("MoveFrom: expected ErrReadOnly, got %v", err)
	}
}

func TestRelease(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")

	if cursor.CheckReleased() {
		t.Fatal("expected not released")
	}

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cursor.Release()

	if !cursor.CheckReleased() {
		t.Fatal("expected released")
	}

	// ops should now report released
	if !ops.CheckReleased() {
		t.Fatal("expected ops released after cursor release")
	}

	_, err = cursor.GetCursorOps(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased, got %v", err)
	}

	_, err = cursor.GetProxyCursor(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased from GetProxyCursor, got %v", err)
	}
}

func TestGetProxyCursor(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	proxy, err := cursor.GetProxyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if proxy != nil {
		t.Fatal("expected nil proxy")
	}
}

func TestGetModTimestamp(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ts, err := ops.GetModTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ts.IsZero() {
		t.Fatalf("expected zero time, got %v", ts)
	}
}

func TestReadAtOnDirectory(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 10)
	_, err = ops.ReadAt(ctx, 0, buf)
	if err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected ErrNotFile, got %v", err)
	}
}

func TestExecutablePermissions(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "build.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	perm, err := childOps.GetPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// executable should have 0755
	if perm&0o111 == 0 {
		t.Fatalf("expected executable bit set, got %s", perm.String())
	}
}

func TestCopyToReturnsNotDone(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	done, err := ops.CopyTo(ctx, nil, "x", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("expected not done")
	}

	done, err = ops.CopyFrom(ctx, "x", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("expected not done")
	}
}

func TestReaddirAllOnFile(t *testing.T) {
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		return nil
	})
	if err != unixfs_errors.ErrNotDirectory {
		t.Fatalf("expected ErrNotDirectory, got %v", err)
	}
}
