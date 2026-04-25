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
	"github.com/go-git/go-git/v6/plumbing/format/objfile"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

func TestDotGitFSCursorRootShape(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ops.GetIsDirectory() {
		t.Fatal("expected root to be directory")
	}
	if ops.GetName() != "" {
		t.Fatalf("expected empty root name, got %q", ops.GetName())
	}

	var names []string
	err = ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"HEAD", "config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}
}

func TestDotGitFSCursorFSHandleRootShape(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	var names []string
	err = handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"HEAD", "config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}

	objects, err := handle.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer objects.Release()

	info, err := objects.GetFileInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected objects file info to be directory")
	}
}

func TestDotGitFSCursorRootLookup(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	child, err := ops.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Release()

	childOps, err := child.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if childOps.GetName() != "objects" {
		t.Fatalf("expected objects name, got %q", childOps.GetName())
	}
	if !childOps.GetIsDirectory() {
		t.Fatal("expected objects to be directory")
	}

	head, err := ops.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer head.Release()

	headOps, err := head.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !headOps.GetIsFile() {
		t.Fatal("expected HEAD to be file")
	}
	size, err := headOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, size)
	if n, err := headOps.ReadAt(ctx, 0, buf); uint64(n) != size || err != nil {
		t.Fatalf("expected full HEAD read, n=%d size=%d err=%v", n, size, err)
	}
	if string(buf) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected HEAD content %q", string(buf))
	}
}

func TestDotGitFSCursorMetadataFiles(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
	if err := store.SetReference(head); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewConfig()
	cfg.Core.IsBare = true
	if err := store.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	expectedConfig, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	cursor := newTestDotGitCursor(t, store)
	defer cursor.Release()

	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	headHandle, err := handle.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()
	if content := readHandleContent(t, ctx, headHandle); string(content) != "ref: refs/heads/main\n" {
		t.Fatalf("unexpected HEAD content %q", string(content))
	}

	configHandle, err := handle.Lookup(ctx, "config")
	if err != nil {
		t.Fatal(err)
	}
	defer configHandle.Release()
	if content := readHandleContent(t, ctx, configHandle); string(content) != string(expectedConfig) {
		t.Fatalf("unexpected config content %q, expected %q", string(content), string(expectedConfig))
	}

	descriptionHandle, err := handle.Lookup(ctx, "description")
	if err != nil {
		t.Fatal(err)
	}
	defer descriptionHandle.Release()
	if content := readHandleContent(t, ctx, descriptionHandle); string(content) != dotGitDefaultDescription {
		t.Fatalf("unexpected description content %q", string(content))
	}
}

func TestDotGitFSCursorRefs(t *testing.T) {
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

	cursor := newTestDotGitCursor(t, store)
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	refsHandle, _, err := handle.LookupPath(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	defer refsHandle.Release()
	if names := readHandleNames(t, ctx, refsHandle); !reflect.DeepEqual(names, []string{"heads", "tags"}) {
		t.Fatalf("unexpected refs entries %v", names)
	}

	headsHandle, _, err := handle.LookupPath(ctx, "refs/heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsHandle.Release()
	if names := readHandleNames(t, ctx, headsHandle); !reflect.DeepEqual(names, []string{"feature", "main"}) {
		t.Fatalf("unexpected heads entries %v", names)
	}

	mainHandle, _, err := handle.LookupPath(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	defer mainHandle.Release()
	if content := readHandleContent(t, ctx, mainHandle); string(content) != mainHash.String()+"\n" {
		t.Fatalf("unexpected main ref content %q", string(content))
	}

	featureHandle, _, err := handle.LookupPath(ctx, "refs/heads/feature/demo")
	if err != nil {
		t.Fatal(err)
	}
	defer featureHandle.Release()
	if content := readHandleContent(t, ctx, featureHandle); string(content) != featureHash.String()+"\n" {
		t.Fatalf("unexpected feature ref content %q", string(content))
	}

	tagsHandle, _, err := handle.LookupPath(ctx, "refs/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer tagsHandle.Release()
	if names := readHandleNames(t, ctx, tagsHandle); !reflect.DeepEqual(names, []string{"v1.0.0"}) {
		t.Fatalf("unexpected tags entries %v", names)
	}
}

func TestDotGitFSCursorLooseObjects(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	hash := storeBlob(t, store, "hello loose object\n")
	prefix := hash.String()[:2]
	suffix := hash.String()[2:]

	cursor := newTestDotGitCursor(t, store)
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	objectsHandle, _, err := handle.LookupPath(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer objectsHandle.Release()
	expectedObjectNames := []string{prefix, "info", "pack"}
	sort.Strings(expectedObjectNames)
	if names := readHandleNames(t, ctx, objectsHandle); !reflect.DeepEqual(names, expectedObjectNames) {
		t.Fatalf("unexpected object prefix entries %v", names)
	}

	prefixHandle, _, err := handle.LookupPath(ctx, "objects/"+prefix)
	if err != nil {
		t.Fatal(err)
	}
	defer prefixHandle.Release()
	if names := readHandleNames(t, ctx, prefixHandle); !reflect.DeepEqual(names, []string{suffix}) {
		t.Fatalf("unexpected object suffix entries %v", names)
	}

	objectHandle, _, err := handle.LookupPath(ctx, "objects/"+prefix+"/"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer objectHandle.Release()
	content := readHandleContent(t, ctx, objectHandle)
	reader, err := objfile.NewReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	typ, size, err := reader.Header()
	if err != nil {
		t.Fatal(err)
	}
	if typ != plumbing.BlobObject || size != int64(len("hello loose object\n")) {
		t.Fatalf("unexpected loose object header type=%v size=%d", typ, size)
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "hello loose object\n" {
		t.Fatalf("unexpected loose object body %q", string(plain))
	}
	if got := reader.Hash(); got != hash {
		t.Fatalf("unexpected loose object hash %s, expected %s", got, hash)
	}
}

func TestDotGitFSCursorGeneratedPlaceholders(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("1111111111111111111111111111111111111111"))
	if err := store.SetReference(ref); err != nil {
		t.Fatal(err)
	}
	shallowHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	if err := store.SetShallow([]plumbing.Hash{shallowHash}); err != nil {
		t.Fatal(err)
	}

	cursor := newTestDotGitCursor(t, store)
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	infoPacksHandle, _, err := handle.LookupPath(ctx, "objects/info/packs")
	if err != nil {
		t.Fatal(err)
	}
	defer infoPacksHandle.Release()
	if content := readHandleContent(t, ctx, infoPacksHandle); string(content) != "\n" {
		t.Fatalf("unexpected objects/info/packs content %q", string(content))
	}

	packHandle, _, err := handle.LookupPath(ctx, "objects/pack")
	if err != nil {
		t.Fatal(err)
	}
	defer packHandle.Release()
	if names := readHandleNames(t, ctx, packHandle); len(names) != 0 {
		t.Fatalf("unexpected objects/pack entries %v", names)
	}

	packedRefsHandle, _, err := handle.LookupPath(ctx, "packed-refs")
	if err != nil {
		t.Fatal(err)
	}
	defer packedRefsHandle.Release()
	expectedRefs := "# pack-refs with: peeled fully-peeled sorted \n" + ref.Hash().String() + " " + ref.Name().String() + "\n"
	if content := readHandleContent(t, ctx, packedRefsHandle); string(content) != expectedRefs {
		t.Fatalf("unexpected packed-refs content %q", string(content))
	}

	shallowHandle, _, err := handle.LookupPath(ctx, "shallow")
	if err != nil {
		t.Fatal(err)
	}
	defer shallowHandle.Release()
	if content := readHandleContent(t, ctx, shallowHandle); string(content) != shallowHash.String()+"\n" {
		t.Fatalf("unexpected shallow content %q", string(content))
	}

	for _, path := range []string{"hooks", "logs", "modules"} {
		dir, _, err := handle.LookupPath(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if names := readHandleNames(t, ctx, dir); len(names) != 0 {
			t.Fatalf("unexpected %s entries %v", path, names)
		}
		dir.Release()
	}
}

func TestDotGitFSCursorEmptyRepository(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	headHandle, _, err := handle.LookupPath(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()
	if content := readHandleContent(t, ctx, headHandle); string(content) != "ref: refs/heads/master\n" {
		t.Fatalf("unexpected empty repository HEAD %q", string(content))
	}

	objectsHandle, _, err := handle.LookupPath(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	defer objectsHandle.Release()
	if names := readHandleNames(t, ctx, objectsHandle); !reflect.DeepEqual(names, []string{"info", "pack"}) {
		t.Fatalf("unexpected empty repository objects entries %v", names)
	}

	headsHandle, _, err := handle.LookupPath(ctx, "refs/heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsHandle.Release()
	if names := readHandleNames(t, ctx, headsHandle); len(names) != 0 {
		t.Fatalf("unexpected empty repository heads entries %v", names)
	}

	tagsHandle, _, err := handle.LookupPath(ctx, "refs/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer tagsHandle.Release()
	if names := readHandleNames(t, ctx, tagsHandle); len(names) != 0 {
		t.Fatalf("unexpected empty repository tags entries %v", names)
	}

	packedRefsHandle, _, err := handle.LookupPath(ctx, "packed-refs")
	if err != nil {
		t.Fatal(err)
	}
	defer packedRefsHandle.Release()
	if content := readHandleContent(t, ctx, packedRefsHandle); string(content) != "# pack-refs with: peeled fully-peeled sorted \n" {
		t.Fatalf("unexpected empty repository packed-refs content %q", string(content))
	}

	shallowHandle, _, err := handle.LookupPath(ctx, "shallow")
	if err != nil {
		t.Fatal(err)
	}
	defer shallowHandle.Release()
	if content := readHandleContent(t, ctx, shallowHandle); len(content) != 0 {
		t.Fatalf("unexpected empty repository shallow content %q", string(content))
	}
}

func TestDotGitFSCursorReleaseRepeatedLookupListingAndOffsetRead(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	var skipped []string
	err = handle.ReaddirAll(ctx, 1, func(ent unixfs.FSCursorDirent) error {
		skipped = append(skipped, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedSkipped := []string{"config", "description", "hooks", "info", "logs", "modules", "objects", "packed-refs", "refs", "shallow"}
	if !reflect.DeepEqual(skipped, expectedSkipped) {
		t.Fatalf("unexpected skipped entries %v", skipped)
	}

	for range 2 {
		headHandle, _, err := handle.LookupPath(ctx, "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 4)
		n, err := headHandle.ReadAt(ctx, 5, buf)
		if n != 4 || err != nil {
			t.Fatalf("expected offset read, n=%d err=%v", n, err)
		}
		if string(buf) != "refs" {
			t.Fatalf("unexpected offset read %q", string(buf))
		}
		tail := make([]byte, 8)
		n, err = headHandle.ReadAt(ctx, int64(len("ref: refs/heads/master\n")-3), tail)
		if n != 3 || err != io.EOF {
			t.Fatalf("expected tail EOF read, n=%d err=%v", n, err)
		}
		headHandle.Release()
	}

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cursor.Release()
	if _, err := ops.GetSize(ctx); err != unixfs_errors.ErrReleased {
		t.Fatalf("expected released GetSize error, got %v", err)
	}
}

func TestDotGitFSCursorMissingAndReadOnly(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage())
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Lookup(ctx, "missing"); err != unixfs_errors.ErrNotExist {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected WriteAt ErrReadOnly, got %v", err)
	}
	if _, err := ops.GetOptimalWriteSize(ctx); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected GetOptimalWriteSize ErrReadOnly, got %v", err)
	}
	if err := ops.SetPermissions(ctx, 0o755, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected SetPermissions ErrReadOnly, got %v", err)
	}
	if err := ops.SetModTimestamp(ctx, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected SetModTimestamp ErrReadOnly, got %v", err)
	}
	if err := ops.Truncate(ctx, 0, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Truncate ErrReadOnly, got %v", err)
	}
	if err := ops.Mknod(ctx, true, []string{"x"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Mknod ErrReadOnly, got %v", err)
	}
	if err := ops.MknodWithContent(ctx, "x", unixfs.NewFSCursorNodeType_File(), 1, bytes.NewReader([]byte("x")), 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MknodWithContent ErrReadOnly, got %v", err)
	}
	if err := ops.Symlink(ctx, true, "x", []string{"HEAD"}, false, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Symlink ErrReadOnly, got %v", err)
	}
	if _, err := ops.MoveTo(ctx, ops, "x", time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MoveTo ErrReadOnly, got %v", err)
	}
	if _, err := ops.MoveFrom(ctx, "x", ops, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected MoveFrom ErrReadOnly, got %v", err)
	}
	if err := ops.Remove(ctx, []string{"HEAD"}, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Remove ErrReadOnly, got %v", err)
	}
}

func TestDotGitFSCursorWritableCapability(t *testing.T) {
	ctx := context.Background()
	cursor := newTestDotGitCursor(t, memory.NewStorage(), WithDotGitWritable(true))
	defer cursor.Release()

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := ops.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected writable directory WriteAt to return ErrNotFile, got %v", err)
	}

	headCursor, err := ops.Lookup(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headCursor.Release()
	headOps, err := headCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := headOps.SetPermissions(ctx, 0o600, time.Time{}); err != ErrDotGitWriteNotImplemented {
		t.Fatalf("expected unsupported child write to inherit writable capability, got %v", err)
	}

	readOnlyCursor := newTestDotGitCursor(t, memory.NewStorage())
	defer readOnlyCursor.Release()
	readOnlyOps, err := readOnlyCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := readOnlyOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected read-only cursor to stay read-only, got %v", err)
	}
}

func TestDotGitFSCursorReferenceWrites(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	mainHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	featureHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	if err := store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash)); err != nil {
		t.Fatal(err)
	}

	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))
	refs, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, err := refs.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	headsRootOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor2, err := headsRootOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	headsOps, err := headsCursor2.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	lockContent := []byte(featureHash.String() + "\n")
	if err := headsOps.MknodWithContent(ctx, "feature.lock", unixfs.NewFSCursorNodeType_File(), int64(len(lockContent)), bytes.NewReader(lockContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if names := readCursorNames(t, ctx, headsOps); !reflect.DeepEqual(names, []string{"feature.lock", "main"}) {
		t.Fatalf("unexpected staged heads entries %v", names)
	}
	lockCursor, err := headsOps.Lookup(ctx, "feature.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockOps, err := lockCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changeCh := make(chan *unixfs.FSCursorChange, 1)
	headsCursor2.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		changeCh <- ch.Clone()
		return false
	})
	done, err := headsOps.MoveFrom(ctx, "feature", lockOps, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected lock rename to complete reference write")
	}
	ref, err := store.Reference(plumbing.NewBranchReferenceName("feature"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Hash() != featureHash {
		t.Fatalf("unexpected feature ref %s", ref.Hash().String())
	}
	if !headsCursor2.CheckReleased() {
		t.Fatal("expected committing cursor to release after ref write")
	}
	select {
	case ch := <-changeCh:
		if !ch.Released {
			t.Fatal("expected released change after ref write")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ref write change callback")
	}

	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refsCursor, err := rootOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, err = refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	headsOps, err = headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := headsOps.Remove(ctx, []string{"feature"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reference(plumbing.NewBranchReferenceName("feature")); err != plumbing.ErrReferenceNotFound {
		t.Fatalf("expected feature ref removed, got %v", err)
	}
}

func TestDotGitFSCursorMetadataWrites(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	mainName := plumbing.NewBranchReferenceName("main")

	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headContent := []byte("ref: " + mainName.String() + "\n")
	if err := rootOps.MknodWithContent(ctx, "HEAD", unixfs.NewFSCursorNodeType_File(), int64(len(headContent)), bytes.NewReader(headContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	head, err := store.Reference(plumbing.HEAD)
	if err != nil {
		t.Fatal(err)
	}
	if head.Target() != mainName {
		t.Fatalf("unexpected HEAD target %q", head.Target().String())
	}

	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	configContent := []byte("[core]\n\tbare = true\n")
	if err := rootOps.MknodWithContent(ctx, "config", unixfs.NewFSCursorNodeType_File(), int64(len(configContent)), bytes.NewReader(configContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Config()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Core.IsBare {
		t.Fatal("expected config core.bare to be true")
	}

	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	shallowHash := plumbing.NewHash("3333333333333333333333333333333333333333")
	shallowContent := []byte(shallowHash.String() + "\n")
	if err := rootOps.MknodWithContent(ctx, "shallow", unixfs.NewFSCursorNodeType_File(), int64(len(shallowContent)), bytes.NewReader(shallowContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	shallows, err := store.Shallow()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shallows, []plumbing.Hash{shallowHash}) {
		t.Fatalf("unexpected shallow hashes %v", shallows)
	}

	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tagHash := plumbing.NewHash("4444444444444444444444444444444444444444")
	packedContent := []byte("# pack-refs with: peeled fully-peeled sorted \n" + tagHash.String() + " refs/tags/v1\n")
	if err := rootOps.MknodWithContent(ctx, "packed-refs", unixfs.NewFSCursorNodeType_File(), int64(len(packedContent)), bytes.NewReader(packedContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	tagRef, err := store.Reference(plumbing.NewTagReferenceName("v1"))
	if err != nil {
		t.Fatal(err)
	}
	if tagRef.Hash() != tagHash {
		t.Fatalf("unexpected packed ref hash %s", tagRef.Hash().String())
	}
}

func TestDotGitFSCursorLooseObjectWrites(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	hash, content := makeLooseObjectContent(t, plumbing.BlobObject, []byte("written loose object\n"))
	prefix := hash.String()[:2]
	suffix := hash.String()[2:]

	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	objectsCursor, err := rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	objectsOps, err := objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := objectsOps.Mknod(ctx, true, []string{prefix}, unixfs.NewFSCursorNodeType_Dir(), 0o755, time.Time{}); err != nil {
		t.Fatal(err)
	}
	prefixCursor, err := objectsOps.Lookup(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}
	prefixOps, err := prefixCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := prefixOps.MknodWithContent(ctx, "tmp_obj_test", unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if names := readCursorNames(t, ctx, prefixOps); !reflect.DeepEqual(names, []string{"tmp_obj_test"}) {
		t.Fatalf("unexpected staged object entries %v", names)
	}
	tmpCursor, err := prefixOps.Lookup(ctx, "tmp_obj_test")
	if err != nil {
		t.Fatal(err)
	}
	tmpOps, err := tmpCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done, err := prefixOps.MoveFrom(ctx, suffix, tmpOps, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected temp object rename to complete")
	}
	if err := store.HasEncodedObject(hash); err != nil {
		t.Fatal(err)
	}
	if !prefixCursor.CheckReleased() {
		t.Fatal("expected object commit to release committing cursor")
	}
}

func TestDotGitFSCursorInvalidWritesLeaveStoreUnchanged(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	mainHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mainHash)); err != nil {
		t.Fatal(err)
	}

	cursor := newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refsCursor, err := rootOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, err := refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	headsOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := headsOps.MknodWithContent(ctx, "main", unixfs.NewFSCursorNodeType_File(), 4, bytes.NewReader([]byte("bad\n")), 0o644, time.Time{}); err == nil {
		t.Fatal("expected malformed ref write to fail")
	}
	ref, err := store.Reference(plumbing.NewBranchReferenceName("main"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Hash() != mainHash {
		t.Fatalf("unexpected main ref after invalid write %s", ref.Hash().String())
	}

	hash, content := makeLooseObjectContent(t, plumbing.BlobObject, []byte("wrong path object\n"))
	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	objectsCursor, err := rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	objectsOps, err := objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := objectsOps.Mknod(ctx, true, []string{"00"}, unixfs.NewFSCursorNodeType_Dir(), 0o755, time.Time{}); err != nil {
		t.Fatal(err)
	}
	prefixCursor, err := objectsOps.Lookup(ctx, "00")
	if err != nil {
		t.Fatal(err)
	}
	prefixOps, err := prefixCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := prefixOps.MknodWithContent(ctx, strings.Repeat("0", 38), unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err == nil {
		t.Fatal("expected mismatched object path write to fail")
	}
	if err := store.HasEncodedObject(hash); err != plumbing.ErrObjectNotFound {
		t.Fatalf("expected object store unchanged after mismatch, got %v", err)
	}

	cursor = newTestDotGitCursor(t, store, WithDotGitWritable(true))
	rootOps, err = cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	objectsCursor, err = rootOps.Lookup(ctx, "objects")
	if err != nil {
		t.Fatal(err)
	}
	objectsOps, err = objectsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	packCursor, err := objectsOps.Lookup(ctx, "pack")
	if err != nil {
		t.Fatal(err)
	}
	packOps, err := packCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := packOps.MknodWithContent(ctx, "pack-test.pack", unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, time.Time{}); err != ErrDotGitWriteNotImplemented {
		t.Fatalf("expected pack writes to be explicitly unsupported, got %v", err)
	}
}

func TestDotGitFSCursorChangeSourceRelease(t *testing.T) {
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

	changeCh := make(chan *unixfs.FSCursorChange, 1)
	cursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		changeCh <- ch.Clone()
		return false
	})

	changeSource.Trigger()

	if !cursor.CheckReleased() {
		t.Fatal("expected cursor to release after repo change")
	}
	if released.Load() != 1 {
		t.Fatal("expected release callback to run exactly once")
	}

	select {
	case ch := <-changeCh:
		if !ch.Released {
			t.Fatal("expected released cursor change")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for cursor release callback")
	}

	if _, err := cursor.GetCursorOps(ctx); err != unixfs_errors.ErrReleased {
		t.Fatalf("expected released cursor ops error, got %v", err)
	}

	changeSource.Trigger()
	if released.Load() != 1 {
		t.Fatal("expected repeated invalidation to stay idempotent")
	}
}

func TestDotGitFSCursorChildReleaseDoesNotReleaseRootOwner(t *testing.T) {
	ctx := context.Background()
	var released atomic.Int32
	cursor := newTestDotGitCursor(
		t,
		memory.NewStorage(),
		WithDotGitReleaseFn(func() {
			released.Add(1)
		}),
	)

	ops, err := cursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := ops.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	child.Release()

	if released.Load() != 0 {
		t.Fatal("expected child release to leave root owner alive")
	}
	if cursor.CheckReleased() {
		t.Fatal("expected root cursor to remain alive after child release")
	}

	cursor.Release()
	if released.Load() != 1 {
		t.Fatal("expected root release to release owner exactly once")
	}
}

func readHandleContent(t *testing.T, ctx context.Context, handle *unixfs.FSHandle) []byte {
	t.Helper()
	size, err := handle.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	n, err := handle.ReadAt(ctx, 0, buf)
	if uint64(n) != size || err != nil {
		t.Fatalf("expected full read, n=%d size=%d err=%v", n, size, err)
	}
	return buf
}

func readHandleNames(t *testing.T, ctx context.Context, handle *unixfs.FSHandle) []string {
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
	t.Helper()
	obj := plumbing.NewMemoryObject(nil)
	obj.SetType(typ)
	obj.SetSize(int64(len(data)))
	writer, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	ow := objfile.NewWriter(&buf)
	if err := ow.WriteHeader(typ, int64(len(data))); err != nil {
		t.Fatal(err)
	}
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
	s.mtx.Lock()
	id := s.nextID
	s.nextID++
	s.cbs[id] = cb
	s.mtx.Unlock()
	return func() {
		s.mtx.Lock()
		delete(s.cbs, id)
		s.mtx.Unlock()
	}
}

func (s *dotGitTestChangeSource) Trigger() {
	s.mtx.Lock()
	cbs := make([]func(), 0, len(s.cbs))
	for _, cb := range s.cbs {
		cbs = append(cbs, cb)
	}
	s.mtx.Unlock()
	for _, cb := range cbs {
		cb()
	}
}
