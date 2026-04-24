package git_block

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	billy "github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	go_git_packfile "github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/sirupsen/logrus"
)

func TestPackfileBytesFileReadAt(t *testing.T) {
	f := newPackfileBytesFile("pack-test.pack", []byte("abcdef"))

	buf := make([]byte, 3)
	n, err := f.ReadAt(buf, 2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != 3 || string(buf) != "cde" {
		t.Fatalf("ReadAt got n=%d data=%q", n, string(buf))
	}

	n, err = f.ReadAt(buf, 4)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if n != 2 || string(buf[:n]) != "ef" {
		t.Fatalf("partial ReadAt got n=%d data=%q", n, string(buf[:n]))
	}
}

func TestStoragePackfileWriter(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)
	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	packData, blobHash := buildTestPackfile(t, []byte("packed data"))
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	obj, err := store.EncodedObject(plumbing.BlobObject, blobHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertObjectData(t, obj, []byte("packed data"))

	if err := store.Commit(); err != nil {
		t.Fatal(err.Error())
	}

	storeRef := store.GetRef()
	oc.SetRootRef(storeRef)
	btx, bcs = oc.BuildTransaction(nil)
	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	obj, err = store.EncodedObject(plumbing.BlobObject, blobHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertObjectData(t, obj, []byte("packed data"))

	packs, err := store.ObjectPacks()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(packs) != 1 {
		t.Fatalf("expected one pack, got %d", len(packs))
	}
}

func TestStoragePackfileWriterReadsCommitTreeAndBlob(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)
	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	packData, commitHash, objectCount := buildTestCommitPackfile(t)
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	commit, err := object.GetCommit(store, commitHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err.Error())
	}
	file, err := tree.File("hello.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	content, err := file.Contents()
	if err != nil {
		t.Fatal(err.Error())
	}
	if content != "hello from pack" {
		t.Fatalf("unexpected file content: %q", content)
	}

	iter, err := store.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer iter.Close()
	var gotCount int
	if err := iter.ForEach(func(plumbing.EncodedObject) error {
		gotCount++
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
	if gotCount != objectCount {
		t.Fatalf("iter count mismatch: got %d want %d", gotCount, objectCount)
	}
}

func buildTestPackfile(t *testing.T, data []byte) ([]byte, plumbing.Hash) {
	t.Helper()

	mem := memory.NewStorage()
	obj := mem.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	wr, err := obj.Writer()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(data); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}
	blobHash, err := mem.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	var buf bytes.Buffer
	enc := go_git_packfile.NewEncoder(&buf, mem, false)
	if _, err := enc.Encode([]plumbing.Hash{blobHash}, 0); err != nil {
		t.Fatal(err.Error())
	}
	return buf.Bytes(), blobHash
}

func buildTestCommitPackfile(t *testing.T) ([]byte, plumbing.Hash, int) {
	t.Helper()

	mem := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(mem, git.WithWorkTree(fs))
	if err != nil {
		t.Fatal(err.Error())
	}
	writeBillyFile(t, fs, "hello.txt", []byte("hello from pack"))
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wt.Add("hello.txt"); err != nil {
		t.Fatal(err.Error())
	}
	commitHash, err := wt.Commit("add hello", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Hydra Test",
			Email: "hydra@example.test",
			When:  time.Unix(1, 0),
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	iter, err := mem.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer iter.Close()

	hashes := make([]plumbing.Hash, 0)
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		hashes = append(hashes, obj.Hash())
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	var buf bytes.Buffer
	enc := go_git_packfile.NewEncoder(&buf, mem, false)
	if _, err := enc.Encode(hashes, 0); err != nil {
		t.Fatal(err.Error())
	}
	return buf.Bytes(), commitHash, len(hashes)
}

func writeBillyFile(t *testing.T, fs billy.Filesystem, name string, data []byte) {
	t.Helper()

	f, err := fs.Create(name)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err.Error())
	}
}

func assertObjectData(t *testing.T, obj plumbing.EncodedObject, expected []byte) {
	t.Helper()

	rc, err := obj.Reader()
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, expected) {
		t.Fatalf("object data mismatch: got %q want %q", data, expected)
	}
}
