package unixfs_tar

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// testTime is a fixed timestamp for test entries.
var testTime = time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)

// tarBuilder is a helper to construct tar archives in memory.
type tarBuilder struct {
	buf bytes.Buffer
	tw  *tar.Writer
}

func newTarBuilder() *tarBuilder {
	b := &tarBuilder{}
	b.tw = tar.NewWriter(&b.buf)
	return b
}

func (b *tarBuilder) addFile(name string, content string, mode int64) {
	b.tw.WriteHeader(&tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     mode,
		ModTime:  testTime,
		Typeflag: tar.TypeReg,
	})
	b.tw.Write([]byte(content))
}

func (b *tarBuilder) addDir(name string, mode int64) {
	b.tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeDir,
		Mode:     mode,
		ModTime:  testTime,
	})
}

func (b *tarBuilder) addSymlink(name, target string) {
	b.tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeSymlink,
		Linkname: target,
		ModTime:  testTime,
	})
}

func (b *tarBuilder) addHardlink(name, target string) {
	b.tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeLink,
		Linkname: target,
		ModTime:  testTime,
	})
}

func (b *tarBuilder) addGlobalHeader() {
	b.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeXGlobalHeader,
		Name:     "pax_global_header",
	})
}

func (b *tarBuilder) finish() *bytes.Reader {
	b.tw.Close()
	return bytes.NewReader(b.buf.Bytes())
}

// buildTestArchive creates a standard test tar archive.
//
// Structure:
//
//	/
//	├── README.md          (0644, "# Hello World\n")
//	├── build.sh           (0755, "#!/bin/sh\necho hello\n")
//	├── docs/              (0755)
//	│   └── guide.txt      (0644, "User guide content")
//	├── empty/             (0755)
//	├── link.txt           -> README.md (symlink)
//	└── src/               (0755)
//	    └── main.go        (0644, "package main\n")
func buildTestArchive() *bytes.Reader {
	// Build the archive directories and their nested files.
	b := newTarBuilder()
	b.addDir("docs/", 0o755)
	b.addFile("docs/guide.txt", "User guide content", 0o644)
	b.addDir("empty/", 0o755)
	b.addDir("src/", 0o755)
	b.addFile("src/main.go", "package main\n", 0o644)

	// Add the root files and symlink to the test archive.
	b.addFile("README.md", "# Hello World\n", 0o644)
	b.addFile("build.sh", "#!/bin/sh\necho hello\n", 0o755)
	b.addSymlink("link.txt", "README.md")
	return b.finish()
}

func mustCursor(t *testing.T, ra *bytes.Reader) *TarFSCursor {
	t.Helper()
	c, err := NewTarFSCursor(ra, int64(ra.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustOps(t *testing.T, c *TarFSCursor) unixfs.FSCursorOps {
	t.Helper()
	ops, err := c.GetCursorOps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestReadFile(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md from the tar root.
	child, err := ops.Lookup(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the README.md file operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify README.md is a regular file.
	if !childOps.GetIsFile() {
		t.Fatal("expected file")
	}

	// Read README.md and verify its content ends at EOF.
	buf := make([]byte, 100)
	n, err := childOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "# Hello World\n" {
		t.Fatalf("unexpected content: %q", string(buf[:n]))
	}
}

func TestReadDir(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Collect the sorted entries from the tar root.
	var names []string
	err := ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the tar root lists every expected entry in order.
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

func TestReadDirSkip(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Collect tar root entries after skipping the first three.
	var names []string
	err := ops.ReaddirAll(ctx, 3, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the remaining tar entries retain their sorted order.
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

func TestLookupNested(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open the docs directory from the tar root.
	docsChild, err := ops.Lookup(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	defer docsChild.Release()

	// Access the docs directory operations.
	docsOps, err := docsChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the docs node retains its directory type and name.
	if !docsOps.GetIsDirectory() {
		t.Fatal("expected docs to be directory")
	}
	if docsOps.GetName() != "docs" {
		t.Fatalf("expected name 'docs', got %q", docsOps.GetName())
	}

	// Open guide.txt from the docs directory.
	guideChild, err := docsOps.Lookup(ctx, "guide.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer guideChild.Release()

	// Access the guide.txt file operations.
	guideOps, err := guideChild.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read guide.txt and verify its archived content.
	buf := make([]byte, 100)
	n, err := guideOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "User guide content" {
		t.Fatalf("unexpected: %q", string(buf[:n]))
	}
}

func TestSymlink(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open link.txt from the tar root.
	child, err := ops.Lookup(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the link.txt symlink operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify link.txt is a symlink instead of a regular file.
	if !childOps.GetIsSymlink() {
		t.Fatal("expected symlink")
	}
	if childOps.GetIsFile() {
		t.Fatal("symlink should not be file")
	}

	// readlink self
	parts, isAbs, err := childOps.Readlink(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if isAbs {
		t.Fatal("expected relative")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkFromDir(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Read link.txt through the tar root and verify its relative target.
	parts, isAbs, err := ops.Readlink(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if isAbs {
		t.Fatal("expected relative")
	}
	if len(parts) != 1 || parts[0] != "README.md" {
		t.Fatalf("expected [README.md], got %v", parts)
	}
}

func TestReadlinkAbsolute(t *testing.T) {
	// Build a tar archive with an absolute symlink target.
	ctx := context.Background()
	b := newTarBuilder()
	b.addSymlink("abs-link", "/usr/local/bin/foo")
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Read the absolute symlink and verify every target component.
	parts, isAbs, err := ops.Readlink(ctx, "abs-link")
	if err != nil {
		t.Fatal(err)
	}
	if !isAbs {
		t.Fatal("expected absolute")
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

func TestHardlink(t *testing.T) {
	// Build a tar archive with a file and its hardlink.
	ctx := context.Background()
	b := newTarBuilder()
	b.addFile("original.txt", "hardlink content", 0o644)
	b.addHardlink("linked.txt", "original.txt")
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open linked.txt from the tar root.
	child, err := ops.Lookup(ctx, "linked.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the linked.txt file operations.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the hardlink resolves to a regular file.
	if !childOps.GetIsFile() {
		t.Fatal("expected hardlink to be a file")
	}

	// Read linked.txt and verify it exposes the original file content.
	buf := make([]byte, 100)
	n, err := childOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "hardlink content" {
		t.Fatalf("unexpected: %q", string(buf[:n]))
	}
}

func TestImplicitDirs(t *testing.T) {
	// Create the tar builder for an entry with implicit parents.
	ctx := context.Background()
	b := newTarBuilder()

	// file with no explicit parent dir entries
	b.addFile("a/b/c/deep.txt", "deep content", 0o644)
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// root should have "a"
	var rootNames []string
	ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		rootNames = append(rootNames, ent.GetName())
		if !ent.GetIsDirectory() {
			t.Fatalf("%q should be directory", ent.GetName())
		}
		return nil
	})
	if len(rootNames) != 1 || rootNames[0] != "a" {
		t.Fatalf("expected [a], got %v", rootNames)
	}

	// navigate to a/b/c/deep.txt
	a, _ := ops.Lookup(ctx, "a")
	defer a.Release()
	aOps, _ := a.GetCursorOps(ctx)

	// Open the implicit b directory beneath a.
	ab, _ := aOps.Lookup(ctx, "b")
	defer ab.Release()
	abOps, _ := ab.GetCursorOps(ctx)

	// Open the implicit c directory beneath a/b.
	abc, _ := abOps.Lookup(ctx, "c")
	defer abc.Release()
	abcOps, _ := abc.GetCursorOps(ctx)

	// Open deep.txt beneath the implicit parent directories.
	deep, _ := abcOps.Lookup(ctx, "deep.txt")
	defer deep.Release()
	deepOps, _ := deep.GetCursorOps(ctx)

	// Read deep.txt and verify its content survived parent creation.
	buf := make([]byte, 100)
	n, err := deepOps.ReadAt(ctx, 0, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "deep content" {
		t.Fatalf("unexpected: %q", string(buf[:n]))
	}
}

func TestPermissions(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// root dir
	perm, err := ops.GetPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if perm&fs.ModeDir == 0 {
		t.Fatal("root should have ModeDir")
	}
	if perm.Perm() != 0o755 {
		t.Fatalf("root: expected 0755, got %o", perm.Perm())
	}

	// regular file
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)
	perm, _ = childOps.GetPermissions(ctx)
	if perm.Perm() != 0o644 {
		t.Fatalf("README.md: expected 0644, got %o", perm.Perm())
	}

	// executable
	exec, _ := ops.Lookup(ctx, "build.sh")
	defer exec.Release()
	execOps, _ := exec.GetCursorOps(ctx)
	perm, _ = execOps.GetPermissions(ctx)
	if perm&0o111 == 0 {
		t.Fatalf("build.sh: expected executable, got %s", perm)
	}
}

func TestModTimestamp(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md and its file operations.
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify README.md retains its archived modification timestamp.
	ts, err := childOps.GetModTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ts.Equal(testTime) {
		t.Fatalf("expected %v, got %v", testTime, ts)
	}
}

func TestReadAtOffset(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md and its file operations.
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// "# Hello World\n" -- skip "# "
	buf := make([]byte, 100)
	n, err := childOps.ReadAt(ctx, 2, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "Hello World\n" {
		t.Fatalf("unexpected: %q", string(buf[:n]))
	}
}

func TestReadAtPastEOF(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md and its file operations.
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify reading beyond README.md returns no bytes and EOF.
	buf := make([]byte, 10)
	n, err := childOps.ReadAt(ctx, 1000, buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes, got %d", n)
	}
}

func TestEmptyArchive(t *testing.T) {
	// Build an empty tar archive.
	ctx := context.Background()
	b := newTarBuilder()
	ra := b.finish()

	// Open the empty archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify the empty tar root is a directory.
	if !ops.GetIsDirectory() {
		t.Fatal("root should be directory")
	}

	// Verify the empty tar root lists no entries.
	var count int
	ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		count++
		return nil
	})
	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
}

func TestGlobalHeader(t *testing.T) {
	// Build a tar archive with a global header and one file.
	ctx := context.Background()
	b := newTarBuilder()
	b.addGlobalHeader()
	b.addFile("file.txt", "content", 0o644)
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify the global header is excluded from directory entries.
	var names []string
	ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if len(names) != 1 || names[0] != "file.txt" {
		t.Fatalf("expected [file.txt], got %v", names)
	}
}

func TestReadOnly(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify tar metadata and file content cannot be changed.
	if err := ops.SetPermissions(ctx, 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("SetPermissions: expected ErrReadOnly, got %v", err)
	}
	if err := ops.SetModTimestamp(ctx, time.Now()); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("SetModTimestamp: expected ErrReadOnly, got %v", err)
	}
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("WriteAt: expected ErrReadOnly, got %v", err)
	}
	if _, err := ops.GetOptimalWriteSize(ctx); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("GetOptimalWriteSize: expected ErrReadOnly, got %v", err)
	}
	if err := ops.Truncate(ctx, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Truncate: expected ErrReadOnly, got %v", err)
	}

	// Verify tar directory entries cannot be created, removed, or moved.
	if err := ops.Mknod(ctx, true, []string{"x"}, nil, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Mknod: expected ErrReadOnly, got %v", err)
	}
	if err := ops.Symlink(ctx, true, "x", []string{"y"}, false, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Symlink: expected ErrReadOnly, got %v", err)
	}
	if err := ops.Remove(ctx, []string{"x"}, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("Remove: expected ErrReadOnly, got %v", err)
	}
	if _, err := ops.MoveTo(ctx, nil, "x", time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("MoveTo: expected ErrReadOnly, got %v", err)
	}
	if _, err := ops.MoveFrom(ctx, "x", nil, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("MoveFrom: expected ErrReadOnly, got %v", err)
	}
}

func TestRelease(t *testing.T) {
	// Open a live tar cursor for release checks.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)

	// Verify the tar cursor begins unreleased.
	if cursor.CheckReleased() {
		t.Fatal("expected not released")
	}

	// Acquire cursor operations before releasing the tar cursor.
	ops := mustOps(t, cursor)
	cursor.Release()

	// Verify release is visible through the cursor and its operations.
	if !cursor.CheckReleased() {
		t.Fatal("expected released")
	}
	if !ops.CheckReleased() {
		t.Fatal("expected ops released")
	}

	// Verify the released cursor rejects operation and proxy access.
	_, err := cursor.GetCursorOps(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased, got %v", err)
	}
	_, err = cursor.GetProxyCursor(ctx)
	if err != unixfs_errors.ErrReleased {
		t.Fatalf("expected ErrReleased from GetProxyCursor, got %v", err)
	}
}

func TestFromReader(t *testing.T) {
	// Build a tar archive for the streaming reader constructor.
	ctx := context.Background()
	b := newTarBuilder()
	b.addFile("test.txt", "from reader", 0o644)
	ra := b.finish()

	// Open the tar reader as a cursor and acquire its operations.
	cursor, err := NewTarFSCursorFromReader(ra)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open test.txt from the buffered tar root.
	child, err := ops.Lookup(ctx, "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Read test.txt and verify the buffered reader content.
	childOps, _ := child.GetCursorOps(ctx)
	buf := make([]byte, 100)
	n, _ := childOps.ReadAt(ctx, 0, buf)
	if string(buf[:n]) != "from reader" {
		t.Fatalf("unexpected: %q", string(buf[:n]))
	}
}

func TestDuplicateEntries(t *testing.T) {
	// Build a tar archive with two entries for file.txt.
	ctx := context.Background()
	b := newTarBuilder()
	b.addFile("file.txt", "first", 0o644)
	b.addFile("file.txt", "second", 0o644)
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open the indexed file.txt entry.
	child, _ := ops.Lookup(ctx, "file.txt")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify file.txt exposes the last archived entry.
	buf := make([]byte, 100)
	n, _ := childOps.ReadAt(ctx, 0, buf)
	if string(buf[:n]) != "second" {
		t.Fatalf("expected last entry wins, got %q", string(buf[:n]))
	}
}

func TestReaddirOnFile(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md and its file operations.
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify a regular file rejects directory enumeration.
	err := childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		return nil
	})
	if err != unixfs_errors.ErrNotDirectory {
		t.Fatalf("expected ErrNotDirectory, got %v", err)
	}
}

func TestReadAtOnDir(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify the tar root rejects file reads.
	buf := make([]byte, 10)
	_, err := ops.ReadAt(ctx, 0, buf)
	if err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected ErrNotFile, got %v", err)
	}
}

func TestLookupNotExist(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify the tar root reports a missing child.
	_, err := ops.Lookup(ctx, "nonexistent")
	if err != unixfs_errors.ErrNotExist {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestReadlinkNotSymlink(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify README.md rejects symlink target reads.
	_, _, err := ops.Readlink(ctx, "README.md")
	if err != unixfs_errors.ErrNotSymlink {
		t.Fatalf("expected ErrNotSymlink, got %v", err)
	}
}

func TestNodeTypes(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Describe the directory entry fields compared by this test.
	type entInfo struct {
		name   string
		isDir  bool
		isFile bool
		isLink bool
	}

	// Collect the tar root entries with their node types.
	var entries []entInfo
	ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entries = append(entries, entInfo{
			name:   ent.GetName(),
			isDir:  ent.GetIsDirectory(),
			isFile: ent.GetIsFile(),
			isLink: ent.GetIsSymlink(),
		})
		return nil
	})

	// Describe the expected file, directory, and symlink entries.
	expected := []entInfo{
		{name: "README.md", isFile: true},
		{name: "build.sh", isFile: true},
		{name: "docs", isDir: true},
		{name: "empty", isDir: true},
		{name: "link.txt", isLink: true},
		{name: "src", isDir: true},
	}

	// Verify the tar directory entries retain their expected node types.
	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(entries))
	}
	for i, e := range expected {
		if entries[i] != e {
			t.Fatalf("entry %d: expected %+v, got %+v", i, e, entries[i])
		}
	}
}

func TestFileSize(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open README.md and its file operations.
	child, _ := ops.Lookup(ctx, "README.md")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify README.md reports its archived byte length.
	size, err := childOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 14 {
		t.Fatalf("expected 14, got %d", size)
	}
}

func TestDirSize(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify the tar root reports a zero directory size.
	size, err := ops.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("expected 0 for dir, got %d", size)
	}
}

func TestEmptyDir(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open the empty directory and its cursor operations.
	child, _ := ops.Lookup(ctx, "empty")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// Verify the empty directory enumerates no entries.
	var count int
	childOps.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		count++
		return nil
	})
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestGetProxyCursor(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()

	// Verify the immutable tar cursor needs no proxy cursor.
	proxy, err := cursor.GetProxyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if proxy != nil {
		t.Fatal("expected nil proxy")
	}
}

func TestCopyReturnsNotDone(t *testing.T) {
	// Open the standard tar archive and its root cursor operations.
	ctx := context.Background()
	ra := buildTestArchive()
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Verify copying out of the tar cursor reports no completed copy.
	done, err := ops.CopyTo(ctx, nil, "x", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("expected not done")
	}

	// Verify copying into the tar cursor reports no completed copy.
	done, err = ops.CopyFrom(ctx, "x", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("expected not done")
	}
}

func TestExactRead(t *testing.T) {
	// Build a tar archive with a five-byte file.
	ctx := context.Background()
	b := newTarBuilder()
	b.addFile("exact.txt", "12345", 0o644)
	ra := b.finish()

	// Open the archive root and its cursor operations.
	cursor := mustCursor(t, ra)
	defer cursor.Release()
	ops := mustOps(t, cursor)

	// Open exact.txt and its file operations.
	child, _ := ops.Lookup(ctx, "exact.txt")
	defer child.Release()
	childOps, _ := child.GetCursorOps(ctx)

	// read exactly the file size
	buf := make([]byte, 5)
	n, err := childOps.ReadAt(ctx, 0, buf)
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}

	// at exact boundary, either nil or EOF is acceptable
	if err != nil && err != io.EOF {
		t.Fatalf("expected nil or io.EOF, got %v", err)
	}
	if string(buf) != "12345" {
		t.Fatalf("unexpected: %q", string(buf))
	}
}
