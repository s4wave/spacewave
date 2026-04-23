package unixfs_git

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/storage/memory"
)

func TestDotGitFSCursorRootShape(t *testing.T) {
	ctx := context.Background()
	cursor := NewDotGitFSCursor(memory.NewStorage(), "")
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

	expected := []string{"HEAD", "config", "description", "hooks", "info", "objects", "refs"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("expected entries %v, got %v", expected, names)
	}
}

func TestDotGitFSCursorFSHandleRootShape(t *testing.T) {
	ctx := context.Background()
	cursor := NewDotGitFSCursor(memory.NewStorage(), "")
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

	expected := []string{"HEAD", "config", "description", "hooks", "info", "objects", "refs"}
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
	cursor := NewDotGitFSCursor(memory.NewStorage(), "")
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

	cursor := NewDotGitFSCursor(store, "")
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

	cursor := NewDotGitFSCursor(store, "")
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

func TestDotGitFSCursorMissingAndReadOnly(t *testing.T) {
	ctx := context.Background()
	cursor := NewDotGitFSCursor(memory.NewStorage(), "")
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
	if err := ops.Mknod(ctx, true, []string{"x"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Mknod ErrReadOnly, got %v", err)
	}
	if err := ops.Remove(ctx, []string{"HEAD"}, time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected Remove ErrReadOnly, got %v", err)
	}
}

func readHandleContent(t *testing.T, ctx context.Context, handle *unixfs.FSHandle) []byte {
	t.Helper()
	size, err := handle.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
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
