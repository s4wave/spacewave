package unixfs_git

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/format/objfile"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

func TestDotGitFSCursorRootShape(t *testing.T) {
	// Open an empty Git repository cursor for root layout checks.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .git root exposes its directory type and empty name.
	if !ops.GetIsDirectory() {
		t.Fatal("expected root to be directory")
	}
	if ops.GetName() != "" {
		t.Fatalf("expected empty root name, got %q", ops.GetName())
	}

	// Enumerate the .git directory names through its cursor.
	var names []string
	err = ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .git root contains the materialized layout entries.
	expected := []string{"HEAD", "config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}
}

func TestDotGitFSCursorFSHandleRootShape(t *testing.T) {
	// Wrap an empty .git cursor in a filesystem handle.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Enumerate the .git root through its filesystem handle.
	var names []string
	err = handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .git root contains the materialized layout entries.
	expected := []string{"HEAD", "config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}

	// Open the objects entry beneath the .git cursor.
	objects, err := handle.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer objects.Release()

	// Read the objects directory file information.
	info, err := objects.GetFileInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .git objects entry is a directory.
	if !info.IsDir() {
		t.Fatal("expected objects file info to be directory")
	}
}

func TestDotGitFSCursorRootLookup(t *testing.T) {
	// Open an empty .git root for child lookup checks.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the objects entry beneath the .git cursor.
	child, err := ops.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	// Access the .git cursor operations for this entry.
	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the objects cursor exposes its directory name and type.
	if childOps.GetName() != "objects" {
		t.Fatalf("expected objects name, got %q", childOps.GetName())
	}
	if !childOps.GetIsDirectory() {
		t.Fatal("expected objects to be directory")
	}

	// Open the HEAD entry beneath the .git cursor.
	head, err := ops.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer head.Release()

	// Access the .git cursor operations for this entry.
	headOps, err := head.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git HEAD is exposed as a file.
	if !headOps.GetIsFile() {
		t.Fatal("expected HEAD to be file")
	}

	// Measure Git HEAD before reading its full content.
	size, err := headOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the complete Git HEAD target.
	buf := make([]byte, size)
	if n, err := headOps.ReadAt(ctx, 0, buf); n != int64(len(buf)) || err != nil {
		t.Fatalf("expected full HEAD read, n=%d size=%d err=%v", n, size, err)
	}

	// Verify Git HEAD uses the default master branch target.
	if string(buf) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected HEAD content %q", string(buf))
	}
}

func TestDotGitFSCursorMetadataFiles(t *testing.T) {
	// Create a Git store with a symbolic HEAD for metadata reads.
	ctx := context.Background()
	store := memory.NewStorage()
	head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
	if err := store.SetReference(head); err != nil {
		t.Fatal(err)
	}

	// Store and serialize the bare Git repository configuration.
	cfg := config.NewConfig()
	cfg.Core.IsBare = true
	if err := store.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	expectedConfig, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	// Open a Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store)
	defer cursor.Release()

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the HEAD entry beneath the .git cursor.
	headHandle, err := handle.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()

	// Verify Git HEAD reflects the stored symbolic target.
	if content := readHandleContent(t, ctx, headHandle); string(content) != "ref: refs/heads/main\n" {
		t.Fatalf("unexpected HEAD content %q", string(content))
	}

	// Open the config entry beneath the .git cursor.
	configHandle, err := handle.Lookup(ctx, "config")
	if err != nil {
		t.Fatal(err)
	}
	defer configHandle.Release()

	// Verify the .git config content matches the stored configuration.
	if content := readHandleContent(t, ctx, configHandle); string(content) != string(expectedConfig) {
		t.Fatalf("unexpected config content %q, expected %q", string(content), string(expectedConfig))
	}

	// Open the description entry beneath the .git cursor.
	descriptionHandle, err := handle.Lookup(ctx, "description")
	if err != nil {
		t.Fatal(err)
	}
	defer descriptionHandle.Release()

	// Verify the .git description uses the default repository text.
	if content := readHandleContent(t, ctx, descriptionHandle); string(content) != dotGitDefaultDescription {
		t.Fatalf("unexpected description content %q", string(content))
	}
}

func TestDotGitFSCursorRefs(t *testing.T) {
	// Store Git branch and tag references for nested directory checks.
	ctx := context.Background()
	store := memory.NewStorage()
	mainHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	featureHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	tagHash := plumbing.NewHash("3333333333333333333333333333333333333333")
	refs := []*plumbing.Reference{
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash),
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature/demo"), featureHash),
		plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), tagHash),
	}
	for _, ref := range refs {
		if err := store.SetReference(ref); err != nil {
			t.Fatal(err)
		}
	}

	// Open a Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store)

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the refs entry beneath the .git cursor.
	refsHandle, _, err := handle.LookupPath(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git refs directory exposes heads and tags.
	defer refsHandle.Release()
	if names := readHandleNames(t, ctx, refsHandle); !reflect.DeepEqual(names, []string{"heads", "tags"}) {
		t.Fatalf("unexpected refs entries %v", names)
	}

	// Open the refs/heads entry beneath the .git cursor.
	headsHandle, _, err := handle.LookupPath(ctx, "refs/heads")
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git branch entries include nested branch directories.
	defer headsHandle.Release()
	if names := readHandleNames(t, ctx, headsHandle); !reflect.DeepEqual(names, []string{"feature", "main"}) {
		t.Fatalf("unexpected heads entries %v", names)
	}

	// Open the refs/heads/main entry beneath the .git cursor.
	mainHandle, _, err := handle.LookupPath(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the main Git branch file contains its hash.
	defer mainHandle.Release()
	if content := readHandleContent(t, ctx, mainHandle); string(content) != mainHash.String()+"\n" {
		t.Fatalf("unexpected main ref content %q", string(content))
	}

	// Open the refs/heads/feature/demo entry beneath the .git cursor.
	featureHandle, _, err := handle.LookupPath(ctx, "refs/heads/feature/demo")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the nested Git branch file contains its hash.
	defer featureHandle.Release()
	if content := readHandleContent(t, ctx, featureHandle); string(content) != featureHash.String()+"\n" {
		t.Fatalf("unexpected feature ref content %q", string(content))
	}

	// Open the refs/tags entry beneath the .git cursor.
	tagsHandle, _, err := handle.LookupPath(ctx, "refs/tags")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git tags directory exposes the stored tag.
	defer tagsHandle.Release()
	if names := readHandleNames(t, ctx, tagsHandle); !reflect.DeepEqual(names, []string{"v1.0.0"}) {
		t.Fatalf("unexpected tags entries %v", names)
	}
}

func TestDotGitFSCursorLooseObjects(t *testing.T) {
	// Store a Git blob and derive its loose-object path.
	ctx := context.Background()
	store := memory.NewStorage()
	hash := storeBlob(t, store, "hello loose object\n")
	prefix := hash.String()[:2]
	suffix := hash.String()[2:]

	// Open a Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store)

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the objects entry beneath the .git cursor.
	objectsHandle, _, err := handle.LookupPath(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer objectsHandle.Release()

	// Verify the Git objects directory includes the loose-object prefix.
	expectedObjectNames := []string{prefix, "info", "pack"}
	sort.Strings(expectedObjectNames)
	if names := readHandleNames(t, ctx, objectsHandle); !reflect.DeepEqual(names, expectedObjectNames) {
		t.Fatalf("unexpected object prefix entries %v", names)
	}

	// Open the objects/ entry beneath the .git cursor.
	prefixHandle, _, err := handle.LookupPath(ctx, "objects/"+prefix)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the loose Git object prefix exposes its suffix file.
	defer prefixHandle.Release()
	if names := readHandleNames(t, ctx, prefixHandle); !reflect.DeepEqual(names, []string{suffix}) {
		t.Fatalf("unexpected object suffix entries %v", names)
	}

	// Open the objects/ entry beneath the .git cursor.
	objectHandle, _, err := handle.LookupPath(ctx, "objects/"+prefix+"/"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer objectHandle.Release()

	// Open the loose Git object content decoder.
	content := readHandleContent(t, ctx, objectHandle)
	reader, err := objfile.NewReader(bytes.NewReader(content), formatcfg.SHA1)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	// Read the loose Git object header.
	typ, size, err := reader.Header()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the loose Git object type and declared size.
	if typ != plumbing.BlobObject || size != int64(len("hello loose object\n")) {
		t.Fatalf("unexpected loose object header type=%v size=%d", typ, size)
	}

	// Read the decoded loose Git object content.
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the loose Git object body and computed hash.
	if string(plain) != "hello loose object\n" {
		t.Fatalf("unexpected loose object body %q", string(plain))
	}
	if got := reader.Hash(); got != hash {
		t.Fatalf("unexpected loose object hash %s, expected %s", got, hash)
	}
}

func TestDotGitFSCursorGeneratedPlaceholders(t *testing.T) {
	// Store a Git branch reference for generated metadata checks.
	ctx := context.Background()
	store := memory.NewStorage()
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("1111111111111111111111111111111111111111"))
	if err := store.SetReference(ref); err != nil {
		t.Fatal(err)
	}

	// Store a Git shallow boundary for generated metadata checks.
	shallowHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	if err := store.SetShallow([]plumbing.Hash{shallowHash}); err != nil {
		t.Fatal(err)
	}

	// Open a Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store)

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the objects/info/packs entry beneath the .git cursor.
	infoPacksHandle, _, err := handle.LookupPath(ctx, "objects/info/packs")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git pack-info placeholder has empty content.
	defer infoPacksHandle.Release()
	if content := readHandleContent(t, ctx, infoPacksHandle); string(content) != "\n" {
		t.Fatalf("unexpected objects/info/packs content %q", string(content))
	}

	// Open the objects/pack entry beneath the .git cursor.
	packHandle, _, err := handle.LookupPath(ctx, "objects/pack")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git pack directory has no entries.
	defer packHandle.Release()
	if names := readHandleNames(t, ctx, packHandle); len(names) != 0 {
		t.Fatalf("unexpected objects/pack entries %v", names)
	}

	// Open the packed-refs entry beneath the .git cursor.
	packedRefsHandle, _, err := handle.LookupPath(ctx, "packed-refs")
	if err != nil {
		t.Fatal(err)
	}
	defer packedRefsHandle.Release()

	// Verify packed Git references contain the stored branch.
	expectedRefs := "# pack-refs with: peeled fully-peeled sorted \n" + ref.Hash().String() + " " + ref.Name().String() + "\n"
	if content := readHandleContent(t, ctx, packedRefsHandle); string(content) != expectedRefs {
		t.Fatalf("unexpected packed-refs content %q", string(content))
	}

	// Open the shallow entry beneath the .git cursor.
	shallowHandle, _, err := handle.LookupPath(ctx, "shallow")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git shallow file contains its boundary hash.
	defer shallowHandle.Release()
	if content := readHandleContent(t, ctx, shallowHandle); string(content) != shallowHash.String()+"\n" {
		t.Fatalf("unexpected shallow content %q", string(content))
	}

	// Verify generated Git placeholder directories remain empty.
	for _, path := range []string{"hooks", "logs", "modules"} {
		// Open the generated Git placeholder directory.
		dir, _, err := handle.LookupPath(ctx, path)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the generated Git placeholder has no entries.
		if names := readHandleNames(t, ctx, dir); len(names) != 0 {
			t.Fatalf("unexpected %s entries %v", path, names)
		}

		// Release the generated Git placeholder handle.
		dir.Release()
	}
}

func TestDotGitFSCursorEmptyRepository(t *testing.T) {
	// Open an empty .git repository through a filesystem handle.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Open the HEAD entry beneath the .git cursor.
	headHandle, _, err := handle.LookupPath(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git repository has a default symbolic HEAD.
	defer headHandle.Release()
	if content := readHandleContent(t, ctx, headHandle); string(content) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected empty repository HEAD %q", string(content))
	}

	// Open the objects entry beneath the .git cursor.
	objectsHandle, _, err := handle.LookupPath(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}

	// Verify empty Git objects retain the info and pack directories.
	defer objectsHandle.Release()
	if names := readHandleNames(t, ctx, objectsHandle); !reflect.DeepEqual(names, []string{"info", "pack"}) {
		t.Fatalf("unexpected empty repository objects entries %v", names)
	}

	// Open the refs/heads entry beneath the .git cursor.
	headsHandle, _, err := handle.LookupPath(ctx, "refs/heads")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git repository has no branch entries.
	defer headsHandle.Release()
	if names := readHandleNames(t, ctx, headsHandle); len(names) != 0 {
		t.Fatalf("unexpected empty repository heads entries %v", names)
	}

	// Open the refs/tags entry beneath the .git cursor.
	tagsHandle, _, err := handle.LookupPath(ctx, "refs/tags")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git repository has no tag entries.
	defer tagsHandle.Release()
	if names := readHandleNames(t, ctx, tagsHandle); len(names) != 0 {
		t.Fatalf("unexpected empty repository tags entries %v", names)
	}

	// Open the packed-refs entry beneath the .git cursor.
	packedRefsHandle, _, err := handle.LookupPath(ctx, "packed-refs")
	if err != nil {
		t.Fatal(err)
	}

	// Verify empty packed Git references contain only the header.
	defer packedRefsHandle.Release()
	if content := readHandleContent(t, ctx, packedRefsHandle); string(content) != "# pack-refs with: peeled fully-peeled sorted \n" {
		t.Fatalf("unexpected empty repository packed-refs content %q", string(content))
	}

	// Open the shallow entry beneath the .git cursor.
	shallowHandle, _, err := handle.LookupPath(ctx, "shallow")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty Git repository has no shallow boundaries.
	defer shallowHandle.Release()
	if content := readHandleContent(t, ctx, shallowHandle); len(content) != 0 {
		t.Fatalf("unexpected empty repository shallow content %q", string(content))
	}
}

func TestDotGitFSCursorReleaseRepeatedLookupListingAndOffsetRead(t *testing.T) {
	// Open an empty Git repository for repeated reads and release checks.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())

	// Wrap the .git root cursor in a filesystem handle.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Enumerate the Git root after skipping its first entry.
	var skipped []string
	err = handle.ReaddirAll(ctx, 1, func(ent unixfs.FSCursorDirent) error {
		skipped = append(skipped, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git root enumeration omits the skipped entry.
	expectedSkipped := []string{"config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(skipped, expectedSkipped) {
		t.Fatalf("unexpected skipped entries %v", skipped)
	}

	// Repeat Git HEAD offset and end-of-file reads through fresh handles.
	for range 2 {
		// Open a fresh Git HEAD handle for each read.
		headHandle, _, err := handle.LookupPath(ctx, "HEAD")
		if err != nil {
			t.Fatal(err)
		}

		// Read four Git HEAD bytes from a nonzero offset.
		buf := make([]byte, 4)
		n, err := headHandle.ReadAt(ctx, 5, buf)

		// Verify the Git HEAD offset read returns the requested bytes.
		if n != 4 || err != nil {
			t.Fatalf("expected offset read, n=%d err=%v", n, err)
		}
		if string(buf) != "refs" {
			t.Fatalf("unexpected offset read %q", string(buf))
		}

		// Read the final Git HEAD bytes into an oversized buffer.
		tail := make([]byte, 8)
		n, err = headHandle.ReadAt(ctx, int64(len("ref: refs/heads/master\n")-3), tail)

		// Verify the Git HEAD tail read reports its length and EOF.
		if n != 3 || err != io.EOF {
			t.Fatalf("expected tail EOF read, n=%d err=%v", n, err)
		}

		// Release the Git HEAD handle after its range reads.
		headHandle.Release()
	}

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Release the Git cursor before checking its existing operations.
	cursor.Release()

	// Verify released Git operations reject content-size reads.
	if _, err := ops.GetSize(ctx); err != unixfs_errors.ErrReleased {
		t.Fatalf("expected released GetSize error, got %v", err)
	}
}

func TestDotGitFSCursorMissingAndReadOnly(t *testing.T) {
	// Open a read-only Git cursor for unsupported operation checks.
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the .git cursor rejects lookup of a missing child.
	if _, err := ops.Lookup(ctx, "missing"); err != unixfs_errors.ErrNotExist {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// Verify read-only Git cursors reject content writes.
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected WriteAt ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject write-size queries.
	if _, err := ops.GetOptimalWriteSize(ctx); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected GetOptimalWriteSize ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject permission changes.
	if err := ops.SetPermissions(ctx, 0o755, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected SetPermissions ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject timestamp changes.
	if err := ops.SetModTimestamp(ctx, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected SetModTimestamp ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject file truncation.
	if err := ops.Truncate(ctx, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Truncate ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject node creation.
	if err := ops.Mknod(ctx, true, []string{"x"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Mknod ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject file creation with content.
	if err := ops.MknodWithContent(ctx, "x", unixfs.NewFSCursorNodeType_File(), 1, bytes.NewReader([]byte("x")), 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MknodWithContent ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject symlink creation.
	if err := ops.Symlink(ctx, true, "x", []string{"HEAD"}, false, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Symlink ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject moves to another cursor.
	if _, err := ops.MoveTo(ctx, ops, "x", time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MoveTo ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject moves from another cursor.
	if _, err := ops.MoveFrom(ctx, "x", ops, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MoveFrom ErrReadOnly, got %v", err)
	}

	// Verify read-only Git cursors reject entry removal.
	if err := ops.Remove(ctx, []string{"HEAD"}, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Remove ErrReadOnly, got %v", err)
	}
}

func TestDotGitFSCursorWritableCapability(t *testing.T) {
	// Open a writable Git cursor to check inherited write capability.
	ctx := context.Background()

	// Open a writable Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, memory.NewStorage(), WithDotGitWritable(true))
	defer cursor.Release()

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify writable Git directories reject file-content writes.
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected writable directory WriteAt to return ErrNotFile, got %v", err)
	}

	// Open the HEAD entry beneath the .git cursor.
	headCursor, err := ops.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headCursor.Release()

	// Access the .git cursor operations for this entry.
	headOps, err := headCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git child cursors inherit unsupported-write behavior.
	if err := headOps.SetPermissions(ctx, 0o600, time.Time{}); err != ErrDotGitWriteNotImplemented {
		t.Fatalf("expected unsupported child write to inherit writable capability, got %v", err)
	}

	// Open a separate read-only Git cursor for capability comparison.
	readOnlyCursor := newTestDotGitCursor(t, memory.NewStorage())
	defer readOnlyCursor.Release()

	// Access the .git cursor operations for this entry.
	readOnlyOps, err := readOnlyCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the separate Git cursor retains read-only behavior.
	if err := readOnlyOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected read-only cursor to stay read-only, got %v", err)
	}
}

func TestDotGitFSCursorReferenceWrites(t *testing.T) {
	// Store the initial Git branch and prepare a new branch hash.
	ctx := context.Background()
	store := memory.NewStorage()
	mainHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	featureHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	if err := store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash)); err != nil {
		t.Fatal(err)
	}

	// Open a writable Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	refs, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the refs entry beneath the .git cursor.
	headsCursor, err := refs.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	headsRootOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the heads entry beneath the .git cursor.
	headsCursor2, err := headsRootOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	headsOps, err := headsCursor2.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Stage the new Git branch hash in a reference lock file.
	lockContent := []byte(featureHash.String() + "\n")
	if err := headsOps.MknodWithContent(ctx, "feature.lock", unixfs.NewFSCursorNodeType_File(), int64(len(lockContent)), bytes.NewReader(lockContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Verify the staged Git reference lock appears beside the branch.
	if names := readCursorNames(t, ctx, headsOps); !reflect.DeepEqual(names, []string{"feature.lock", "main"}) {
		t.Fatalf("unexpected staged heads entries %v", names)
	}

	// Open the feature.lock entry beneath the .git cursor.
	lockCursor, err := headsOps.Lookup(ctx, "feature.lock")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	lockOps, err := lockCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe to Git reference cursor release changes.
	changeCh := make(chan *unixfs.FSCursorChange, 1)
	headsCursor2.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		changeCh <- ch.Clone()
		return false
	})

	// Publish the Git branch by moving its staged reference lock.
	done, err := headsOps.MoveFrom(ctx, "feature", lockOps, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git reference lock move completes the write.
	if !done {
		t.Fatal("expected lock rename to complete reference write")
	}

	// Read the published Git branch from the store.
	ref, err := store.Reference(plumbing.NewBranchReferenceName("feature"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git branch hash and committing cursor release.
	if ref.Hash() != featureHash {
		t.Fatalf("unexpected feature ref %s", ref.Hash().String())
	}
	if !headsCursor2.CheckReleased() {
		t.Fatal("expected committing cursor to release after ref write")
	}

	// Wait for the Git reference cursor release notification.
	select {
	case ch := <-changeCh:
		if !ch.Released {
			t.Fatal("expected released change after ref write")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ref write change callback")
	}

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the refs entry beneath the .git cursor.
	refsCursor, err := rootOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the heads entry beneath the .git cursor.
	headsCursor, err = refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	headsOps, err = headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the published Git branch through its cursor.
	if err := headsOps.Remove(ctx, []string{"feature"}, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Verify the removed Git branch is absent from the store.
	if _, err := store.Reference(plumbing.NewBranchReferenceName("feature")); err != plumbing.ErrReferenceNotFound {
		t.Fatalf("expected feature ref removed, got %v", err)
	}
}

func TestDotGitFSCursorMetadataWrites(t *testing.T) {
	// Create a Git store and the target branch name for metadata writes.
	ctx := context.Background()
	store := memory.NewStorage()
	mainName := plumbing.NewBranchReferenceName("main")

	// Open a writable Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write a symbolic Git HEAD through the .git root cursor.
	headContent := []byte("ref: " + mainName.String() + "\n")
	if err := rootOps.MknodWithContent(ctx, "HEAD", unixfs.NewFSCursorNodeType_File(), int64(len(headContent)), bytes.NewReader(headContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Read the persisted Git HEAD reference.
	head, err := store.Reference(plumbing.HEAD)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git HEAD targets the requested branch.
	if head.Target() != mainName {
		t.Fatalf("unexpected HEAD target %q", head.Target().String())
	}

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write the bare repository configuration through the .git cursor.
	configContent := []byte("[core]\n\tbare = true\n")
	if err := rootOps.MknodWithContent(ctx, "config", unixfs.NewFSCursorNodeType_File(), int64(len(configContent)), bytes.NewReader(configContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Read the persisted Git repository configuration.
	cfg, err := store.Config()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git configuration retains the bare flag.
	if !cfg.Core.IsBare {
		t.Fatal("expected config core.bare to be true")
	}

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write the Git shallow boundary through the .git cursor.
	shallowHash := plumbing.NewHash("3333333333333333333333333333333333333333")
	shallowContent := []byte(shallowHash.String() + "\n")
	if err := rootOps.MknodWithContent(ctx, "shallow", unixfs.NewFSCursorNodeType_File(), int64(len(shallowContent)), bytes.NewReader(shallowContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Read the persisted Git shallow boundaries.
	shallows, err := store.Shallow()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git shallow boundary hash matches the written content.
	if !reflect.DeepEqual(shallows, []plumbing.Hash{shallowHash}) {
		t.Fatalf("unexpected shallow hashes %v", shallows)
	}

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write a Git tag through packed-refs content.
	tagHash := plumbing.NewHash("4444444444444444444444444444444444444444")
	packedContent := []byte("# pack-refs with: peeled fully-peeled sorted \n" + tagHash.String() + " refs/tags/v1\n")
	if err := rootOps.MknodWithContent(ctx, "packed-refs", unixfs.NewFSCursorNodeType_File(), int64(len(packedContent)), bytes.NewReader(packedContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Read the persisted Git tag reference.
	tagRef, err := store.Reference(plumbing.NewTagReferenceName("v1"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git tag hash matches the packed reference.
	if tagRef.Hash() != tagHash {
		t.Fatalf("unexpected packed ref hash %s", tagRef.Hash().String())
	}
}

func TestDotGitFSCursorLooseObjectWrites(t *testing.T) {
	// Encode a Git blob and derive its destination loose-object path.
	ctx := context.Background()
	store := memory.NewStorage()
	hash, content := makeLooseObjectContent(t, plumbing.BlobObject, []byte("written loose object\n"))
	prefix := hash.String()[:2]
	suffix := hash.String()[2:]

	// Open a writable Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the objects entry beneath the .git cursor.
	objectsCursor, err := rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	objectsOps, err := objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Stage the Git loose-object prefix directory.
	if err := objectsOps.Mknod(ctx, true, []string{prefix}, unixfs.NewFSCursorNodeType_Dir(), 0o755, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Open the loose-object prefix entry beneath the .git cursor.
	prefixCursor, err := objectsOps.Lookup(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	prefixOps, err := prefixCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Stage the encoded Git object in a temporary file.
	if err := prefixOps.MknodWithContent(ctx, "tmp_obj_test", unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Verify the staged Git temporary object appears in its directory.
	if names := readCursorNames(t, ctx, prefixOps); !reflect.DeepEqual(names, []string{"tmp_obj_test"}) {
		t.Fatalf("unexpected staged object entries %v", names)
	}

	// Open the tmp_obj_test entry beneath the .git cursor.
	tmpCursor, err := prefixOps.Lookup(ctx, "tmp_obj_test")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	tmpOps, err := tmpCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the loose Git object by moving its temporary file.
	done, err := prefixOps.MoveFrom(ctx, suffix, tmpOps, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Git temporary object move completes publication.
	if !done {
		t.Fatal("expected temp object rename to complete")
	}

	// Verify the Git store contains the published object.
	if err := store.HasEncodedObject(hash); err != nil {
		t.Fatal(err)
	}

	// Verify the Git object commit releases its cursor.
	if !prefixCursor.CheckReleased() {
		t.Fatal("expected object commit to release committing cursor")
	}
}

func TestDotGitFSCursorInvalidWritesLeaveStoreUnchanged(t *testing.T) {
	// Store an initial Git branch for invalid-write checks.
	ctx := context.Background()
	store := memory.NewStorage()
	mainHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash)); err != nil {
		t.Fatal(err)
	}

	// Open a writable Git repository cursor for the stored state.
	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the refs entry beneath the .git cursor.
	refsCursor, err := rootOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the heads entry beneath the .git cursor.
	headsCursor, err := refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	headsOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify malformed Git reference content is rejected.
	if err := headsOps.MknodWithContent(ctx, "main", unixfs.NewFSCursorNodeType_File(), 4, bytes.NewReader([]byte("bad\n")), 0o644, time.Time{}); err == nil {
		t.Fatal("expected malformed ref write to fail")
	}

	// Read the Git branch after the malformed write.
	ref, err := store.Reference(plumbing.NewBranchReferenceName("main"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the malformed write preserves the original Git branch hash.
	if ref.Hash() != mainHash {
		t.Fatalf("unexpected main ref after invalid write %s", ref.Hash().String())
	}

	// Encode a Git blob for a mismatched loose-object path.
	hash, content := makeLooseObjectContent(t, plumbing.BlobObject, []byte("wrong path object\n"))

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the objects entry beneath the .git cursor.
	objectsCursor, err := rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	objectsOps, err := objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Stage a Git object directory with a mismatched hash prefix.
	if err := objectsOps.Mknod(ctx, true, []string{"00"}, unixfs.NewFSCursorNodeType_Dir(), 0o755, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Open the 00 entry beneath the .git cursor.
	prefixCursor, err := objectsOps.Lookup(ctx, "00")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	prefixOps, err := prefixCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify a Git object write rejects a mismatched destination hash.
	if err := prefixOps.MknodWithContent(ctx, strings.Repeat("0", 38), unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err == nil {
		t.Fatal("expected mismatched object path write to fail")
	}

	// Verify the rejected Git object is absent from the store.
	if err := store.HasEncodedObject(hash); err != plumbing.ErrObjectNotFound {
		t.Fatalf("expected object store unchanged after mismatch, got %v", err)
	}

	// Open a writable Git repository cursor for the stored state.
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))

	// Access the .git cursor operations for this entry.
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the objects entry beneath the .git cursor.
	objectsCursor, err = rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	objectsOps, err = objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the pack entry beneath the .git cursor.
	packCursor, err := objectsOps.Lookup(ctx, "pack")
	if err != nil {
		t.Fatal(err)
	}

	// Access the .git cursor operations for this entry.
	packOps, err := packCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Git pack-file writes remain explicitly unsupported.
	if err := packOps.MknodWithContent(ctx, "pack-test.pack", unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err != ErrDotGitWriteNotImplemented {
		t.Fatalf("expected pack writes to be explicitly unsupported, got %v", err)
	}
}

func TestDotGitFSCursorChangeSourceRelease(t *testing.T) {
	// Open a Git cursor with change notifications and a counted release callback.
	ctx := context.Background()
	var released atomic.Int32
	changeSource := newDotGitTestChangeSource()
	cursor := newTestDotGitCursor(
		t,
		memory.NewStorage(),
		WithDotGitChangeSource(changeSource),
		WithDotGitReleaseFn(func() {
			released.Add(1)
		}),
	)

	// Subscribe to the Git cursor release notification.
	changeCh := make(chan *unixfs.FSCursorChange, 1)
	cursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		changeCh <- ch.Clone()
		return false
	})

	// Invalidate the Git repository through its change source.
	changeSource.Trigger()

	// Verify Git invalidation releases the cursor and resource once.
	if !cursor.CheckReleased() {
		t.Fatal("expected cursor to release after repo change")
	}
	if released.Load() != 1 {
		t.Fatal("expected release callback to run exactly once")
	}

	// Wait for the Git cursor release notification.
	select {
	case ch := <-changeCh:
		if !ch.Released {
			t.Fatal("expected released cursor change")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for cursor release callback")
	}

	// Verify invalidated Git cursors reject access to operations.
	if _, err := cursor.GetCursorOps(ctx); err != unixfs_errors.ErrReleased {
		t.Fatalf("expected released cursor ops error, got %v", err)
	}

	// Repeat Git invalidation to check release idempotence.
	changeSource.Trigger()

	// Verify repeated Git invalidation keeps the release count unchanged.
	if released.Load() != 1 {
		t.Fatal("expected repeated invalidation to stay idempotent")
	}
}

func TestDotGitFSCursorChildReleaseDoesNotReleaseRootOwner(t *testing.T) {
	// Open a Git root cursor with a counted resource release callback.
	ctx := context.Background()
	var released atomic.Int32
	cursor := newTestDotGitCursor(
		t,
		memory.NewStorage(),
		WithDotGitReleaseFn(func() {
			released.Add(1)
		}),
	)

	// Access the .git cursor operations for this entry.
	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the refs entry beneath the .git cursor.
	child, err := ops.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}

	// Release the Git child cursor before checking the root resource.
	child.Release()

	// Verify Git child release preserves the root resource and cursor.
	if released.Load() != 0 {
		t.Fatal("expected child release to leave root owner alive")
	}
	if cursor.CheckReleased() {
		t.Fatal("expected root cursor to remain alive after child release")
	}

	// Release the Git root cursor and verify its resource closes once.
	cursor.Release()
	if released.Load() != 1 {
		t.Fatal("expected root release to release owner exactly once")
	}
}

func readHandleContent(t *testing.T, ctx context.Context, handle *unixfs.FSHandle) []byte {
	// Measure the Git filesystem handle content before reading it.
	t.Helper()
	size, err := handle.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Use empty content for zero-length Git files.
	if size == 0 {
		return nil
	}

	// Read the complete Git filesystem handle content.
	buf := make([]byte, size)
	n, err := handle.ReadAt(ctx, 0, buf)

	// Verify the Git handle read fills the buffer without an error.
	if n != int64(len(buf)) || err != nil {
		t.Fatalf("expected full read, n=%d size=%d err=%v", n, size, err)
	}
	return buf
}

func readHandleNames(t *testing.T, ctx context.Context, handle *unixfs.FSHandle) []string {
	// Collect directory names through the Git filesystem handle.
	t.Helper()
	var names []string
	err := handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func readCursorNames(t *testing.T, ctx context.Context, ops unixfs.FSCursorOps) []string {
	// Collect directory names through the Git cursor operations.
	t.Helper()
	var names []string
	err := ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func makeLooseObjectContent(t *testing.T, typ plumbing.ObjectType, data []byte) (plumbing.Hash, []byte) {
	// Prepare a Git memory object and its content writer.
	t.Helper()
	obj := plumbing.NewMemoryObject(nil)
	obj.SetType(typ)
	obj.SetSize(int64(len(data)))
	writer, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}

	// Write and close the Git memory object content.
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	// Encode the loose Git object header into a buffer.
	var buf bytes.Buffer
	ow := objfile.NewWriter(&buf, formatcfg.SHA1)
	if err := ow.WriteHeader(typ, int64(len(data))); err != nil {
		t.Fatal(err)
	}

	// Write and finish the loose Git object content stream.
	if _, err := ow.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := ow.Close(); err != nil {
		t.Fatal(err)
	}
	return obj.Hash(), buf.Bytes()
}

type dotGitTestTx struct {
	storage.Storer
	readOnly bool
}

func (t *dotGitTestTx) Commit(ctx context.Context) error { return nil }

func (t *dotGitTestTx) Discard() {}

func (t *dotGitTestTx) GetReadOnly() bool { return t.readOnly }

func newTestDotGitCursor(t *testing.T, storer storage.Storer, opts ...DotGitFSCursorOption) *DotGitFSCursor {
	t.Helper()
	return NewDotGitFSCursorWithOptions(&dotGitTestTx{Storer: storer}, "", opts...)
}

type dotGitTestChangeSource struct {
	mtx    sync.Mutex
	nextID int
	cbs    map[int]func()
}

func newDotGitTestChangeSource() *dotGitTestChangeSource {
	return &dotGitTestChangeSource{
		cbs: make(map[int]func()),
	}
}

func (s *dotGitTestChangeSource) AddDotGitChangeCb(cb func()) func() {
	// Register the Git change callback under a fresh identity.
	s.mtx.Lock()
	id := s.nextID
	s.nextID++
	s.cbs[id] = cb
	s.mtx.Unlock()

	// Return the unsubscribe function for this Git change callback.
	return func() {
		s.mtx.Lock()
		delete(s.cbs, id)
		s.mtx.Unlock()
	}
}

func (s *dotGitTestChangeSource) Trigger() {
	// Snapshot Git change callbacks under the change-source mutex.
	s.mtx.Lock()
	cbs := make([]func(), 0, len(s.cbs))
	for _, cb := range s.cbs {
		cbs = append(cbs, cb)
	}
	s.mtx.Unlock()

	// Notify the snapshot of Git change subscribers outside the mutex.
	for _, cb := range cbs {
		cb()
	}
}
