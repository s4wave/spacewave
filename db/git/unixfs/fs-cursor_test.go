package unixfs_git

import (
	"context"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// storeBlob writes a blob to the storage and returns its hash.
func storeBlob(t *testing.T, s *memory.Storage, content string) plumbing.Hash {
	// Prepare a Git blob and open its content writer.
	t.Helper()
	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}

	// Write and close the Git blob content.
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Store the Git blob and return its hash.
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// storeTree writes a tree to the storage and returns its hash.
func storeTree(t *testing.T, s *memory.Storage, entries []object.TreeEntry) plumbing.Hash {
	// Encode the sorted Git tree entries.
	t.Helper()
	sort.Sort(object.TreeEntrySorter(entries))
	tree := &object.Tree{Entries: entries}
	obj := s.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		t.Fatal(err)
	}

	// Store the encoded Git tree and return its hash.
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
	// Mark Git tree fixture failures at the calling test.
	t.Helper()

	// Store the Git fixture file and symlink blobs.
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

	// Load the completed Git root tree fixture.
	tree, err := object.GetTree(s, rootHash)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestReaddirAll(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git root entries match the fixture order.
	if !ops.GetIsDirectory() {
		t.Fatal("expected root to be a directory")
	}
	if ops.GetName() != "" {
		t.Fatalf("expected empty name for root, got %q", ops.GetName())
	}

	// Collect the Git directory entries through the cursor.
	var names []string
	err = ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git root entries match the fixture order.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Collect the Git directory entries through the cursor.
	var names []string
	err = ops.ReaddirAll(ctx, 3, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git enumeration skips the requested entries.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Define the Git entry properties captured during enumeration.
	type entInfo struct {
		name   string
		isDir  bool
		isFile bool
		isLink bool
	}

	// Collect the Git directory entries through the cursor.
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

	// Describe the expected Git entry types.
	expected := []entInfo{
		{name: "README.md", isFile: true},
		{name: "build.sh", isFile: true},
		{name: "docs", isDir: true},
		{name: "empty", isDir: true},
		{name: "link.txt", isLink: true},
		{name: "src", isDir: true},
	}

	// Verify the Git entries retain their file, directory, and symlink types.
	for i, e := range expected {
		if entries[i] != e {
			t.Fatalf("entry %d: expected %+v, got %+v", i, e, entries[i])
		}
	}
}

func TestLookupFile(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify README.md is exposed as a named Git file.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the full file and verify the end-of-file contract.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read from a non-zero offset and verify the returned suffix.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the requested Git content range through the cursor.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the Git entry content size.
	size, err := childOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git file size matches its blob content.
	if size != 14 { // "# Hello World\n" = 14 bytes
		t.Fatalf("expected size 14, got %d", size)
	}
}

func TestDirectorySize(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the Git entry content size.
	size, err := ops.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git directory reports zero content bytes.
	if size != 0 {
		t.Fatalf("expected directory size 0, got %d", size)
	}
}

func TestFilePermissions(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Describe the expected Git regular and executable permissions.
	tests := []struct {
		name string

		perm string // expected permission string representation
	}{
		{"README.md", "-rw-r--r--"},
		{"build.sh", "-rwxr-xr-x"},
	}

	// Read and check each Git file permission case.
	for _, tt := range tests {
		// Open the Git file named by the permission case.
		child, err := ops.Lookup(ctx, tt.name)
		if err != nil {
			t.Fatalf("lookup %s: %v", tt.name, err)
		}

		// Access the Git file cursor operations.
		childOps, err := child.GetCursorOps(ctx)
		if err != nil {
			t.Fatalf("getops %s: %v", tt.name, err)
		}

		// Read the Git file permissions.
		perm, err := childOps.GetPermissions(ctx)
		if err != nil {
			t.Fatalf("getperm %s: %v", tt.name, err)
		}

		// Verify the Git file permissions match the case.
		if perm.String() != tt.perm {
			t.Fatalf("%s: expected permissions %s, got %s", tt.name, tt.perm, perm.String())
		}

		// Release the Git file cursor after its permission check.
		child.Release()
	}
}

func TestRootPermissions(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the Git entry filesystem permissions.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up nonexistent.txt in the Git tree.
	_, err = ops.Lookup(ctx, "nonexistent.txt")
	if err != unixfs_errors.ErrNotExist {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestSubdirectoryNavigation(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
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

	// Access the Git child cursor operations.
	docsOps, err := docsChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git subtree entry type and name.
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

	// Access the Git child cursor operations.
	guideOps, err := guideChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git subtree entry type and name.
	if !guideOps.GetIsFile() {
		t.Fatal("expected guide.txt to be a file")
	}

	// Read the nested Git guide file content.
	buf := make([]byte, 100)
	n, err := guideOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Verify the nested Git guide content matches its blob.
	if string(buf[:n]) != "User guide content" {
		t.Fatalf("unexpected content: %q", string(buf[:n]))
	}
}

func TestEmptyDirectory(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up empty in the Git tree.
	child, err := ops.Lookup(ctx, "empty")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git subtree has no entries.
	if !childOps.GetIsDirectory() {
		t.Fatal("expected empty to be a directory")
	}

	// Collect the Git directory entries through the cursor.
	var count int
	err = childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git subtree has no entries.
	if count != 0 {
		t.Fatalf("expected 0 entries in empty dir, got %d", count)
	}
}

func TestSymlinkLookup(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up link.txt in the Git tree.
	child, err := ops.Lookup(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git symlink is neither a file nor a directory.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up link.txt in the Git tree.
	child, err := ops.Lookup(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the Git symlink target through the cursor.
	parts, isAbsolute, err := childOps.Readlink(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git symlink cursor returns its relative target.
	if isAbsolute {
		t.Fatal("expected relative link")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkFromDir(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the Git symlink target through the cursor.
	parts, isAbsolute, err := ops.Readlink(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git directory resolves the relative symlink target.
	if isAbsolute {
		t.Fatal("expected relative link")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkAbsolute(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()

	// Build a symlink with an absolute target for readlink checks.
	linkHash := storeBlob(t, s, "/usr/local/bin/foo")
	rootHash := storeTree(t, s, []object.TreeEntry{
		{Name: "abs-link", Mode: filemode.Symlink, Hash: linkHash},
	})

	// Load the Git root tree containing the absolute symlink.
	tree, err := object.GetTree(s, rootHash)
	if err != nil {
		t.Fatal(err)
	}

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the Git symlink target through the cursor.
	parts, isAbsolute, err := ops.Readlink(ctx, "abs-link")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git symlink retains its absolute target components.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the Git symlink target through the cursor.
	_, _, err = ops.Readlink(ctx, "README.md")
	if err != unixfs_errors.ErrNotSymlink {
		t.Fatalf("expected ErrNotSymlink, got %v", err)
	}
}

func TestWriteOpsReturnReadOnly(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
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

	// Confirm immutable cursor operations reject removal.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")

	// Verify the Git cursor starts live.
	if cursor.CheckReleased() {
		t.Fatal("expected not released")
	}

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Release the Git cursor before checking access failures.
	cursor.Release()

	// Verify the Git cursor reports its released state.
	if !cursor.CheckReleased() {
		t.Fatal("expected released")
	}

	// ops should now report released
	if !ops.CheckReleased() {
		t.Fatal("expected ops released after cursor release")
	}

	// Access the Git child cursor operations.
	_, err = cursor.GetCursorOps(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased, got %v", err)
	}

	// Read the Git cursor proxy result.
	_, err = cursor.GetProxyCursor(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased from GetProxyCursor, got %v", err)
	}
}

func TestGetProxyCursor(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Read the Git cursor proxy result.
	proxy, err := cursor.GetProxyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify immutable Git cursors have no proxy cursor.
	if proxy != nil {
		t.Fatal("expected nil proxy")
	}
}

func TestGetModTimestamp(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the Git entry modification timestamp.
	ts, err := ops.GetModTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git entries report an unset modification timestamp.
	if !ts.IsZero() {
		t.Fatalf("expected zero time, got %v", ts)
	}
}

func TestReadAtOnDirectory(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the requested Git content range through the cursor.
	buf := make([]byte, 10)
	_, err = ops.ReadAt(ctx, 0, buf)
	if err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected ErrNotFile, got %v", err)
	}
}

func TestExecutablePermissions(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up build.sh in the Git tree.
	child, err := ops.Lookup(ctx, "build.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the Git entry filesystem permissions.
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
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Request a copy from the immutable Git cursor.
	done, err := ops.CopyTo(ctx, nil, "x", time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify unsupported Git copies report no completed operation.
	if done {
		t.Fatal("expected not done")
	}

	// Request a copy into the immutable Git cursor.
	done, err = ops.CopyFrom(ctx, "x", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify unsupported Git copies report no completed operation.
	if done {
		t.Fatal("expected not done")
	}
}

func TestReaddirAllOnFile(t *testing.T) {
	// Create the in-memory Git tree fixture for cursor checks.
	ctx := context.Background()
	s := memory.NewStorage()
	tree := buildTestTree(t, s)

	// Open the Git root cursor for the fixture.
	cursor := NewGitFSCursor(s, tree, "")
	defer cursor.Release()

	// Access the Git root cursor operations.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Look up README.md in the Git tree.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the Git child cursor operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Collect the Git directory entries through the cursor.
	err = childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		return nil
	})
	if err != unixfs_errors.ErrNotDirectory {
		t.Fatalf("expected ErrNotDirectory, got %v", err)
	}
}
